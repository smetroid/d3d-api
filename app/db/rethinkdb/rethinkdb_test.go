package rethinkdb

import (
	"testing"

	"github.com/smetroid/d3d-api/app/models"
)

func TestRethinkDB_CRUD_DAG(t *testing.T) {
	db := getTestDB(t)

	nodes := &models.Node{}
	edges := &models.Edge{}
	dag := &models.Dag{
		Title:       "dagre_test",
		Description: "This is my first test",
		Edges:       nodes,
		Nodes:       edges,
	}
	dag.GenerateDefaults()

	//Create a new DAG
	id, err := db.CreateDAG(*dag)
	if err != nil {
		t.Fatal(err)
	}

}

// docker run -d --name rethinkdb -p 8080:8080 -p 28015:28015 rethinkdb
func getTestDB(t *testing.T) (db *RethinkDB) {
	db = &RethinkDB{}
	err := db.Init()

	if err != nil {
		t.Fatal(err)
	}

	return db
}
