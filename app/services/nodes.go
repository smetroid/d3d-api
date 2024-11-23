package services

import (
	"log"

	"github.com/smetroid/d3d-api/app/db/rethinkdb"
	"github.com/smetroid/d3d-api/app/models"
)

type NodeService struct {
	DB *rethinkdb.RethinkDB
}

func (ds *NodeService) GetNode(id string) (nodeResponse models.NodeResponse, err error) {
	node, err := ds.DB.GetNode(id)
	nodeResponse = models.NewNodeResponse(node)
	return
}

func (ds *NodeService) GetNodes(queryParams map[string][]string) (nodesResponse models.NodesResponse, err error) {
	nodes, err := ds.DB.GetNodesSummary(queryParams)
	if err != nil {
		return
	}
	nodesResponse = models.NewNodesResponse(nodes)

	return
}

func (ds *NodeService) DeleteNode(id string) (err error) {
	err = ds.DB.DeleteNode(id)
	return
}

func (ds *NodeService) ProcessNode(currentNode models.Node) (id string, err error) {
	currentNode.GenerateDefaults()
	existingNode, foundExistingNode, err := ds.DB.FindRelatedNode(currentNode)

	if !foundExistingNode {
		//new node
		id, err = ds.DB.CreateNode(currentNode)
		if err != nil {
			log.Println(err)
		}
		return
	}

	log.Println(existingNode)

	return
}
