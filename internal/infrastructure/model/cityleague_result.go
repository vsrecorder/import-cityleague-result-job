package model

import (
	"time"
)

type CityleagueResult struct {
	CityleagueScheduleId string
	OfficialEventId      uint
	LeagueType           uint
	EventDate            time.Time
	PlayerId             string
	PlayerName           string
	Rank                 uint
	Point                uint
	DeckCode             string
}

func NewCityleagueResult(
	cityleagueScheduleId string,
	officialEventId uint,
	leagueType uint,
	eventDate time.Time,
	playerId string,
	playerName string,
	rank uint,
	point uint,
	deckCode string,
) *CityleagueResult {
	return &CityleagueResult{
		CityleagueScheduleId: cityleagueScheduleId,
		OfficialEventId:      officialEventId,
		LeagueType:           leagueType,
		EventDate:            eventDate,
		PlayerId:             playerId,
		PlayerName:           playerName,
		Rank:                 rank,
		Point:                point,
		DeckCode:             deckCode,
	}
}
