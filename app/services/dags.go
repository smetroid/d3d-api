package services

import (
	"log"

	"github.com/smetroid/d3d-api/app/db/rethinkdb"
	"github.com/smetroid/d3d-api/app/models"
)

type DAGService struct {
	DB *rethinkdb.RethinkDB
}

func (ds *DAGService) GetDAG(id string) (dagResponse models.DAGResponse, err error) {
	dag, err := ds.DB.GetDAG(id)
	dagResponse = models.NewDAGResponse(dag)
	return
}

func (ds *DAGService) GetDAGs(queryParams map[string][]string) (dagsResponse models.DAGsResponse, err error) {
	dags, err := ds.DB.GetDAGsSummary(queryParams)
	if err != nil {
		return
	}
	dagsResponse = models.NewDAGsResponse(dags)

	return
}

func (ds *DAGService) DeleteDAG(id string) (err error) {
	err = ds.DB.DeleteDAG(id)
	return
}

func (ds *DAGService) ProcessDAG(currentDAG models.Dag) (id string, err error) {
	currentDAG.GenerateDefaults()
	existingDAG, foundExistingDAG, err := ds.DB.FindRelatedDAG(currentDAG)

	if !foundExistingDAG {
		//new DAG
		id, err = ds.DB.CreateDAG(currentDAG)
		if err != nil {
			log.Println(err)
		}
		return
	}

	id = existingDAG.Id

	return
}

func (ds *DAGService) UpdateDAG(id string, dag models.Dag) (err error) {
	err = ds.DB.UpdateDAG(id, dag)
	return
}
