package services

import (
	"log"

	"github.com/smetroid/d3d-api/app/db/rethinkdb"
	"github.com/smetroid/d3d-api/app/models"
)

type EdgeService struct {
	DB *rethinkdb.RethinkDB
}

func (ds *EdgeService) GetEdge(id string) (edgeResponse models.EdgeResponse, err error) {
	edge, err := ds.DB.GetEdge(id)
	edgeResponse = models.NewEdgeResponse(edge)
	return
}

func (ds *EdgeService) GetEdges(queryParams map[string][]string) (edgesResponse models.EdgesResponse, err error) {
	edges, err := ds.DB.GetEdgesSummary(queryParams)
	if err != nil {
		return
	}
	edgesResponse = models.NewEdgesResponse(edges)

	return
}

func (ds *EdgeService) DeleteEdge(id string) (err error) {
	err = ds.DB.DeleteEdge(id)
	return
}

func (ds *EdgeService) ProcessEdge(currentEdge models.Edge) (id string, err error) {
	currentEdge.GenerateDefaults()
	existingEdge, foundExistingEdge, err := ds.DB.FindRelatedEdge(currentEdge)

	if !foundExistingEdge {
		//new Edge
		id, err = ds.DB.CreateEdge(currentEdge)
		if err != nil {
			log.Println(err)
		}
		return
	}

	log.Println(existingEdge)

	return
}
