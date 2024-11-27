package models

import (
	"time"

	"github.com/google/uuid"
)

type Dag struct {
	//globally unique random UUID
	Id string `gorethink:"id,opmitempty" json:"id"`

	//Title
	Name string `gorethink:"name" json:"name"`

	//Dag diagram long description
	Description string `gorethink:"description" json:"description"`

	//list of edges and nodes for the diagram
	Diagram string `gorethink:"diagram" json:"diagram"`

	//UTC date and time the diagram was generated in ISO 8601 format
	Created time.Time `gorethink:"created" json:"created"`

	Updated time.Time `gorethink:"updated" json:"updated"`
}

type DAGResponse struct {
	Status      string    `json:"status"`
	LastTime    time.Time `json:"lastTime"`
	AutoRefresh bool      `json:"autoRefresh"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Diagram     string    `json:"diagram"`
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

func (dag *Dag) GenerateDefaults() {
	if dag.Id == "" {
		id := uuid.Must(uuid.NewRandom())
		dag.Id = id.String()
	}

	if dag.Created.IsZero() {
		dag.Created = time.Now()
	}
}
