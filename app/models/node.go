package models

import (
	"time"

	"github.com/google/uuid"
)

type Node struct {
	//globally unique random UUID
	Id string `gorethink:"id,opmitempty" json:"id"`
	//Id string `gorethink:"id,opmitempty"`

	V                    string            `gorethink:"v json:"v"`
	Parent               string            `gorethink:"parent" json:"parent"`
	ValueLabel           map[string]string `gorethink:"valueLabel" json:"value.label"`
	ValueType            string            `gorethink:"labelType json:"value.labeltype"`
	ValueClusterLabelPos string            `gorethink:"clusterLabelPos json:"value.clusterlabelpos"`
	ValueStyle           string            `gorethink:"clusterStyle json:"value.style"`
	//UTC date and time the alert was generated in ISO 8601 format
	CreateTime time.Time `gorethink:"createTime" json:"createTime"`
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

	if node.CreateTime.IsZero() {
		node.CreateTime = time.Now()
	}
}
