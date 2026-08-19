package models

import (
	"time"

	"github.com/google/uuid"
)

type Dag struct {
	//globally unique random UUID
	Id string `db:"id" json:"id"`

	//Title
	Name string `db:"name" json:"name"`

	//Dag diagram long description
	Description string `db:"description" json:"description"`

	//list of edges and nodes for the diagram
	Diagram string `db:"diagram" json:"diagram"`

	//UTC date and time the diagram was generated in ISO 8601 format
	Created time.Time `db:"created" json:"created"`

	Updated time.Time `db:"updated" json:"updated"`

	// ClientId is used for WS echo prevention — not persisted to DB.
	ClientId string `db:"-" json:"clientId,omitempty"`

	// Public controls whether this diagram can be fetched without authentication.
	Public bool `db:"public" json:"public"`

	// EmbedRevision increments on every content save for cache-busting.
	EmbedRevision int64 `db:"embed_revision" json:"embedRevision"`
}

type DagHistory struct {
	Id           string    `db:"id" json:"id"`
	DagId        string    `db:"dag_id" json:"dagId"`
	SnapshotJSON string    `db:"snapshot_json" json:"snapshotJson"`
	SavedBy      string    `db:"saved_by" json:"savedBy"`
	SavedAt      time.Time `db:"saved_at" json:"savedAt"`
}

type DagHistoryResponse struct {
	Status  string       `json:"status"`
	History []DagHistory `json:"history"`
}

type DAGResponse struct {
	Status      string    `json:"status"`
	LastTime    time.Time `json:"lastTime"`
	AutoRefresh bool      `json:"autoRefresh"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Diagram     string    `json:"diagram"`
}

// DagPublicResponse is the shape returned by GET /dag/:id/public.
// It omits owner/ACL fields; only public-safe fields are exposed.
type DagPublicResponse struct {
	Status        string `json:"status"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Diagram       string `json:"diagram"`
	EmbedRevision int64  `json:"embedRevision"`
}

type DAGsResponse struct {
	Status      string                   `json:"status"`
	Dags        []map[string]interface{} `json:"dags"`
	LastTime    time.Time                `json:"lastTime"`
	AutoRefresh bool                     `json:"autoRefresh"`
	Total       int                      `json:"total"`
}

func NewDAGResponse(dag Dag) (dr DAGResponse) {
	dr = DAGResponse{}
	dr.Status = "ok"
	dr.AutoRefresh = true
	dr.Name = dag.Name
	dr.Description = dag.Description
	dr.Diagram = dag.Diagram

	//dr.Total = len(dag)
	return
}

func NewDAGsResponse(dags []map[string]interface{}) (dr DAGsResponse) {
	dr = DAGsResponse{}
	dr.Dags = dags
	dr.Status = "ok"
	dr.AutoRefresh = false
	dr.Total = len(dags)
	return
}

func NewDagPublicResponse(dag Dag) DagPublicResponse {
	return DagPublicResponse{
		Status:        "ok",
		Name:          dag.Name,
		Description:   dag.Description,
		Diagram:       dag.Diagram,
		EmbedRevision: dag.EmbedRevision,
	}
}

func (dag *Dag) GenerateDefaults() {
	if dag.Id == "" {
		id := uuid.Must(uuid.NewRandom())
		dag.Id = id.String()
	}

	if dag.Created.IsZero() {
		dag.Created = time.Now()
	}
}
