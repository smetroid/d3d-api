package models

import (
	"time"

	"github.com/google/uuid"
)

type Node struct {
	//globally unique random UUID
	Id string `db:"id" json:"id"`

	V                    string            `db:"v" json:"v"`
	Parent               string            `db:"parent" json:"parent"`
	ValueLabel           map[string]string `db:"value_label" json:"value.label"`
	ValueType            string            `db:"value_type" json:"value.labeltype"`
	ValueClusterLabelPos string            `db:"value_cluster_label_pos" json:"value.clusterlabelpos"`
	ValueStyle           string            `db:"value_style" json:"value.style"`
	//UTC date and time the alert was generated in ISO 8601 format
	Created time.Time `db:"created" json:"created"`
}

type NodeResponse struct {
	Status      string    `json:"status"`
	LastTime    time.Time `json:"lastTime"`
	AutoRefresh bool      `json:"autoRefresh"`
	Total       int       `json:"total"`
}

type NodesResponse struct {
	Status      string                   `json:"status"`
	Nodes       []map[string]interface{} `json:"nodes"`
	LastTime    time.Time                `json:"lastTime"`
	AutoRefresh bool                     `json:"autoRefresh"`
	Total       int                      `json:"total"`
}

func NewNodeResponse(node Node) (nr NodeResponse) {
	nr = NodeResponse{}
	nr.Status = "ok"
	nr.AutoRefresh = true
	return
}

func NewNodesResponse(nodes []map[string]interface{}) (nr NodesResponse) {
	nr = NodesResponse{}
	nr.Nodes = nodes
	nr.Status = "ok"
	nr.AutoRefresh = false
	nr.Total = len(nodes)
	return
}

func (node *Node) GenerateDefaults() {
	if node.Id == "" {
		id := uuid.Must(uuid.NewRandom())
		node.Id = id.String()
	}

	if node.Created.IsZero() {
		node.Created = time.Now()
	}
}
