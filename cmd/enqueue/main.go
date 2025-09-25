package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/vsrecorder/import-cityleague-result-job/internal/infrastructure/simplemq"
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

func getEvents(date time.Time) ([]*OfficialEvent, error) {
	startDateYear := uint16(date.Year())
	startDateMonth := uint8(date.Month())
	startDateDay := uint8(date.Day())

	endDateYear := uint16(date.Year())
	endDateMonth := uint8(date.Month())
	endDateDay := uint8(date.Day())

	res, err := http.Get(fmt.Sprintf(
		"https://beta.vsrecorder.mobi/api/v1beta/official_events?type_id=2&league_type=0&start_date=%d-%02d-%02d&end_date=%d-%02d-%02d",
		startDateYear, startDateMonth, startDateDay, endDateYear, endDateMonth, endDateDay),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var oegr OfficialEventGetResponse
	if err := json.Unmarshal(body, &oegr); err != nil {
		return nil, err
	}

	return oegr.OfficialEvents, nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if err := godotenv.Load(); err != nil {
		log.Printf("Failed to load .env file: %v", err)
		os.Exit(1)
	}

	mqName := os.Getenv("MQ_NAME")
	mqToken := os.Getenv("MQ_TOKEN")
	mqc := simplemq.NewSimpleMQClient(mqName, mqToken)

	date := time.Now()

	var events []*OfficialEvent
	events, err := getEvents(date)
	if err != nil {
		log.Printf("Failed to get events for date %s: %v", date.Format("2006-01-02"), err)
		os.Exit(1)
	}

	for _, event := range events {
		v, err := json.Marshal(*event)
		if err != nil {
			log.Printf("Failed to marshal event to JSON: %v", err)
			os.Exit(1)
		}

		msgReq := &simplemq.SendMessageRequest{
			Content: string(base64.StdEncoding.EncodeToString(v)),
		}

		if _, err := mqc.SendMessage(context.Background(), msgReq); err != nil {
			log.Printf("Failed to send message to MQ: %v", err)
			os.Exit(1)
		}
	}

	os.Exit(0)
}
