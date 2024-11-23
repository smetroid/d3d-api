package models

import (
	"time"

	"github.com/google/uuid"
)

type Edge struct {
	//globally unique random UUID
	Id string `gorethink:"id,opmitempty" json:"id"`
	//UTC date and time the alert was generated in ISO 8601 format
	CreateTime time.Time         `gorethink:"createTime" json:"createTime"`
	V          string            `gorethink:"v" json:"v"`
	W          string            `gorethink:"w" json:"w"`
	Label      map[string]string `gorethink:"label" json:"value.label"`
}

type EdgeResponse struct {
	Status      string    `json:"status"`
	LastTime    time.Time `json:"lastTime"`
	AutoRefresh bool      `json:"autoRefresh"`
	Total       int       `json:"total"`
}

type EdgesResponse struct {
	Status      string                   `json:"status"`
	Edges       []map[string]interface{} `json:"edges"`
	LastTime    time.Time                `json:"lastTime"`
	AutoRefresh bool                     `json:"autoRefresh"`
	Total       int                      `json:"total"`
}

func NewEdgeResponse(edge Edge) (er EdgeResponse) {
	er = EdgeResponse{}
	er.Status = "ok"
	er.AutoRefresh = true
	return
}

func NewEdgesResponse(edges []map[string]interface{}) (er EdgesResponse) {
	er = EdgesResponse{}
	er.Edges = edges
	er.Status = "ok"
	er.AutoRefresh = false
	er.Total = len(edges)
	return
}

func (edge *Edge) GenerateDefaults() {
	if edge.Id == "" {
		id := uuid.Must(uuid.NewRandom())
		edge.Id = id.String()
	}

	if edge.CreateTime.IsZero() {
		edge.CreateTime = time.Now()
	}
}
