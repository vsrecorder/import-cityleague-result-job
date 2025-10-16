package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/joho/godotenv"
	"github.com/vsrecorder/import-cityleague-result-job/internal/infrastructure/model"
	"github.com/vsrecorder/import-cityleague-result-job/internal/infrastructure/postgres"
	"github.com/vsrecorder/import-cityleague-result-job/internal/infrastructure/simplemq"
)

const (
	errorMaxNum       = 50
	concurrencyMaxNum = 100
)

type OfficialEvent struct {
	ID              uint      `json:"id"`
	Title           string    `json:"title"`
	Address         string    `json:"address"`
	Venue           string    `json:"venue"`
	Date            time.Time `json:"date"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	TypeName        string    `json:"type_name"`
	LeagueTitle     string    `json:"league_title"`
	RegulationTitle string    `json:"regulation_title"`
	CSPFlg          bool      `json:"csp_flg"`
	Capacity        uint      `json:"capacity"`
	ShopId          uint      `json:"shop_id"`
	ShopName        string    `json:"shop_name"`
}

type OfficialEventGetResponse struct {
	TypeId         uint             `json:"type_id"`
	LeagueType     uint             `json:"league_type"`
	StartDate      time.Time        `json:"start_date"`
	EndDate        time.Time        `json:"end_date"`
	OfficialEvents []*OfficialEvent `json:"official_events"`
}

type EventResultDetailSearch struct {
	Code    uint           `json:"code"`
	Count   uint           `json:"count"`
	Results []*EventResult `json:"results"`
}

type EventResult struct {
	PlayerId string `json:"player_id"`
	Name     string `json:"name"`
	Rank     uint   `json:"rank"`
	Point    uint   `json:"point"`
	DeckId   string `json:"deck_id"`
}

func getEventResults(eventId uint) ([]*EventResult, error) {
	res, err := http.Get(fmt.Sprintf(
		"https://players.pokemon-card.com/event_result_detail_search?event_holding_id=%d",
		eventId),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return []*EventResult{}, nil
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var eds EventResultDetailSearch
	if err := json.Unmarshal(body, &eds); err != nil {
		return nil, err
	}

	return eds.Results, nil
}

func convertPNG2JPG(imageBytes []byte) ([]byte, error) {
	contentType := http.DetectContentType(imageBytes)

	switch contentType {
	case "image/png":
		img, err := png.Decode(bytes.NewReader(imageBytes))
		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("unable to convert %#v to jpeg", contentType)
}

func uploadDeckImage(deckCode string) error {
	ctx := context.Background()
	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	baseEndpoint := "https://s3.isk01.sakurastorage.jp"
	s3client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = &baseEndpoint
	})

	// すでにアップロードされている場合はスキップする
	if _, err = s3client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("vsrecorder"),
		Key:    aws.String(fmt.Sprintf("images/decks/%s.jpg", deckCode)),
	}); err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			url := fmt.Sprintf("https://www.pokemon-card.com/deck/deckView.php/deckID/%s.png", deckCode)

			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			srcImg, _, err := image.Decode(resp.Body)
			if err != nil {
				return err
			}

			var w bytes.Buffer
			err = png.Encode(&w, srcImg)
			if err != nil {
				return err
			}

			imageBytes, err := convertPNG2JPG(w.Bytes())
			if err != nil {
				return err
			}

			if _, err = s3client.PutObject(ctx, &s3.PutObjectInput{
				ACL:    "public-read",
				Bucket: aws.String("vsrecorder"),
				Key:    aws.String(fmt.Sprintf("images/decks/%s.jpg", deckCode)),
				Body:   bytes.NewReader(imageBytes),
			}); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

// エラーチャンネルを使用してゴルーチンからのエラーを受け取る
type workerError struct {
	err      error
	exitCode int
	message  string
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Failed to load .env file: %v", err)
		os.Exit(1)
	}

	if _, err := awsConfig.LoadDefaultConfig(context.Background()); err != nil {
		log.Printf("Failed to load default aws config: %v", err)
		os.Exit(1)
	}

	dbHostname := os.Getenv("DB_HOSTNAME")
	dbPort := os.Getenv("DB_PORT")
	userName := os.Getenv("DB_USER_NAME")
	userPassword := os.Getenv("DB_USER_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	mqName := os.Getenv("MQ_NAME")
	mqToken := os.Getenv("MQ_TOKEN")

	db, err := postgres.NewDB(dbHostname, dbPort, userName, userPassword, dbName)
	if err != nil {
		log.Printf("Failed to load connect database: %v", err)
		os.Exit(1)
	}

	mqc := simplemq.NewSimpleMQClient(mqName, mqToken)

	errorChan := make(chan workerError, errorMaxNum)
	semChan := make(chan struct{}, concurrencyMaxNum)

	var wg sync.WaitGroup
	for {
		res, err := mqc.ReceiveMessage(context.Background())
		if err != nil {
			log.Printf("Failed to receive message from MQ: %v", err)
			// TODO: リトライ処理を入れるなど
			continue
		}

		if len(res.Messages) == 0 {
			break
		}

		msg := res.Messages[0]

		v, err := base64.StdEncoding.DecodeString(msg.Content)
		if err != nil {
			log.Printf("Invalid base64 in message %v, skipping: %v", msg.ID, err)
			// TODO: デッドレターキューに入れるなど
			continue
		}

		var event OfficialEvent
		if err := json.Unmarshal(v, &event); err != nil {
			log.Printf("Invalid JSON in message %v, skipping: %v", msg.ID, err)
			// TODO: デッドレターキューに入れるなど
			continue
		}

		{
			semChan <- struct{}{}
			wg.Add(1)
			go func(event OfficialEvent, msgId string) {
				defer func() {
					wg.Done()
					<-semChan
				}()

				// ゴルーチン内でのpanic保護
				defer func() {
					if r := recover(); r != nil {
						errorChan <- workerError{
							err:      fmt.Errorf("panic recovered: %v", r),
							exitCode: 1,
							message:  "Unexpected panic occurred in worker goroutine",
						}
					}
				}()

				var leagueType uint
				switch event.LeagueTitle {
				case "オープン":
					leagueType = 1
				case "ジュニア":
					leagueType = 2
				case "シニア":
					leagueType = 3
				case "マスター":
					leagueType = 4
				default:
					leagueType = 0
				}

				// イベントの結果を取得
				results, err := getEventResults(event.ID)
				if err != nil {
					select {
					case errorChan <- workerError{
						err:      err,
						exitCode: 1,
						message:  fmt.Sprintf("Failed to get event results for event ID %d", event.ID),
					}:
					default:
					}
					return
				}

				if len(results) == 0 {
					// 結果がない場合はスキップ
					log.Printf("No results found for event ID %d, skipping", event.ID)
					return
				}

				// 対象期間中のシティーリーグのIDを取得する
				var cs model.CityleagueSchedule
				if tx := db.Where("from_date <= ? AND to_date >= ?", event.Date, event.Date).First(&cs); tx.Error != nil {
					select {
					case errorChan <- workerError{
						err:      tx.Error,
						exitCode: 1,
						message:  fmt.Sprintf("Failed to find cityleague schedule for date %v", event.Date),
					}:
					default:
					}
					return
				}

				cityleagueScheduleId := cs.ID

				for _, result := range results {
					// デッキコードがある場合は画像をアップロードする
					if result.DeckId != "" {
						if err := uploadDeckImage(result.DeckId); err != nil {
							select {
							case errorChan <- workerError{
								err:      err,
								exitCode: 1,
								message:  fmt.Sprintf("Failed to upload deck image for deck ID %s", result.DeckId),
							}:
							default:
							}
							return
						}
					}

					m := model.NewCityleagueResult(
						cityleagueScheduleId,
						event.ID,
						leagueType,
						event.Date,
						result.PlayerId,
						result.Name,
						result.Rank,
						result.Point,
						result.DeckId,
					)

					log.Printf("CityleagueResult: %v", m)

					if tx := db.Create(&m); tx.Error != nil {
						var pgErr *pgconn.PgError
						if errors.As(tx.Error, &pgErr) && pgErr.Code == "23505" {
							continue
						} else {
							select {
							case errorChan <- workerError{
								err:      tx.Error,
								exitCode: 1,
								message:  fmt.Sprintf("Failed to insert cityleague result for player ID %s", result.PlayerId),
							}:
							default:
							}
							return
						}
					}
				}

				// キューから削除
				if err := mqc.DeleteMessage(context.Background(), msgId); err != nil {
					select {
					case errorChan <- workerError{
						err:      err,
						exitCode: 1,
						message:  fmt.Sprintf("Failed to delete message %v from MQ", msgId),
					}:
					default:
					}
					return
				}
			}(event, msg.ID)
		}
	}

	// エラー集約
	go func() {
		wg.Wait()
		close(errorChan)
	}()

	for workerErr := range errorChan {
		log.Printf("%s: %v", workerErr.message, workerErr.err)
	}

	os.Exit(0)
}
