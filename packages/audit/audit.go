package audit

import "time"

type Event struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ActorID        string    `json:"actor_id"`
	Action         string    `json:"action"`
	TargetType     string    `json:"target_type"`
	TargetID       string    `json:"target_id"`
	Result         string    `json:"result"`
	Metadata       any       `json:"metadata"`
	Timestamp      time.Time `json:"timestamp"`
}
