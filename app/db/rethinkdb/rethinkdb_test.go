package rethinkdb

import (
	"testing"

	"github.com/smetroid/d3d-api/app/models"
)

func TestRethinkDB_CRUD_DAG(t *testing.T) {
	db := getTestDB(t)

	dag := &models.Dag{
		Name:        "dagre_test",
		Description: "This is my first test",
	}
	dag.GenerateDefaults()

	//Create a new DAG
	_, err := db.CreateDAG(*dag)
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
