package rethinkdb

import (
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/smetroid/d3d-api/app/models"
	r "gopkg.in/gorethink/gorethink.v4"
)

type RethinkDB struct {
	Address  string `toml:"address"`
	Database string `toml:"database"`
	session  *r.Session
}

func (re *RethinkDB) Init() error {

	if re.Address == "" {
		re.Address = "localhost:28015"
	}
	if re.Database == "" {
		re.Database = "samus_test"
	}

	return re.connect()
}

func (re *RethinkDB) connect() error {
	session, err := r.Connect(r.ConnectOpts{
		Address: re.Address,
	})
	if err != nil {
		return err
	}
	re.session = session

	err = re.createDBIfNotExist()
	if err != nil {
		return err
	}

	err = re.createTableIfNotExist("dags")
	if err != nil {
		return err
	}

	err = re.createTableIfNotExist("dag_history")
	if err != nil {
		return err
	}

	err = re.createIndexIfNotExist("dag_history", "dag_id")
	if err != nil {
		return err
	}

	if err = re.InitSharesTables(); err != nil {
		return err
	}

	return nil
}

func (re *RethinkDB) createDBIfNotExist() error {
	exists, err := re.dbExists()
	if err != nil {
		return err
	}

	if !exists {
		_, err := r.DBCreate(re.Database).RunWrite(re.session)
		if err != nil {
			return err
		}
	}

	return nil
}

func (re *RethinkDB) createTableIfNotExist(table string) error {
	exists, err := re.tableExists(table)
	if err != nil {
		return err
	}

	if !exists {
		_, err := r.DB(re.Database).TableCreate(table).RunWrite(re.session)
		if err != nil {
			return err
		}
	}

	return nil
}

func (re *RethinkDB) dbExists() (bool, error) {
	var response []interface{}
	res, err := r.DBList().Run(re.session)

	if err != nil {
		return false, err
	}

	err = res.All(&response)
	if err != nil {
		return false, err
	}

	for _, db := range response {
		if db == re.Database {
			return true, nil
		}
	}

	return false, nil
}

func (re *RethinkDB) tableExists(table string) (bool, error) {
	var response []interface{}
	res, err := r.DB(re.Database).TableList().Run(re.session)

	if err != nil {
		return false, err
	}

	err = res.All(&response)
	if err != nil {
		return false, err
	}

	for _, responseTable := range response {
		if responseTable == table {
			return true, nil
		}
	}

	return false, nil
}

// Create DAG and return generated id
func (re *RethinkDB) CreateDAG(dag models.Dag) (string, error) {
	ids, err := re.CreateDAGs([]models.Dag{dag})

	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return dag.Id, nil
	}
	return ids[0], nil
}

func (re *RethinkDB) CreateEdge(edge models.Edge) (string, error) {
	ids, err := re.CreateEdges([]models.Edge{edge})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return edge.Id, nil
	}
	return ids[0], nil
}

func (re *RethinkDB) CreateNode(node models.Node) (string, error) {
	ids, err := re.CreateNodes([]models.Node{node})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return node.Id, nil
	}
	return ids[0], nil
}

func (re *RethinkDB) CreateMenu(menu models.Menu) (string, error) {
	ids, err := re.CreateMenus([]models.Menu{menu})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return menu.Id, nil
	}
	return ids[0], nil
}

// Create DAG and return generated ids
func (re *RethinkDB) CreateDAGs(dag []models.Dag) (ids []string, err error) {
	writeResponse, err := r.DB(re.Database).Table("dags").Insert(dag).RunWrite(re.session)
	if err != nil {
		return ids, err
	}
	return writeResponse.GeneratedKeys, nil
}

func (re *RethinkDB) CreateEdges(edge []models.Edge) (ids []string, err error) {
	writeResponse, err := r.DB(re.Database).Table("edges").Insert(edge).RunWrite(re.session)
	if err != nil {
		return ids, err
	}
	return writeResponse.GeneratedKeys, nil
}

func (re *RethinkDB) CreateNodes(node []models.Node) (ids []string, err error) {
	writeResponse, err := r.DB(re.Database).Table("nodes").Insert(node).RunWrite(re.session)
	if err != nil {
		return ids, err
	}
	return writeResponse.GeneratedKeys, nil
}

func (re *RethinkDB) CreateMenus(menu []models.Menu) (ids []string, err error) {
	writeResponse, err := r.DB(re.Database).Table("menus").Insert(menu).RunWrite(re.session)
	if err != nil {
		return ids, err
	}
	return writeResponse.GeneratedKeys, nil
}

func (re *RethinkDB) findDAGs(filter interface{}) (dag []models.Dag, err error) {
	res, err := r.DB(re.Database).Table("dags").Filter(filter).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&dag)
	if dag == nil {
		dag = []models.Dag{}
	}
	return
}

func (re *RethinkDB) findEdges(filter interface{}) (edge []models.Edge, err error) {
	res, err := r.DB(re.Database).Table("edges").Filter(filter).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&edge)
	if edge == nil {
		edge = []models.Edge{}
	}
	return
}

func (re *RethinkDB) findNodes(filter interface{}) (node []models.Node, err error) {
	res, err := r.DB(re.Database).Table("nodes").Filter(filter).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&node)
	if node == nil {
		node = []models.Node{}
	}
	return
}

func (re *RethinkDB) findMenus(filter interface{}) (menu []models.Menu, err error) {
	res, err := r.DB(re.Database).Table("menus").Filter(filter).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&menu)
	if menu == nil {
		menu = []models.Menu{}
	}
	return
}

func (re *RethinkDB) GetDAG(id string) (dag models.Dag, err error) {
	res, err := r.DB(re.Database).Table("dags").Get(id).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.One(&dag)
	return
}

func (re *RethinkDB) GetEdge(id string) (edge models.Edge, err error) {
	res, err := r.DB(re.Database).Table("edges").Get(id).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.One(&edge)
	return
}

func (re *RethinkDB) GetNode(id string) (node models.Node, err error) {
	res, err := r.DB(re.Database).Table("nodes").Get(id).Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	err = res.One(&node)
	return
}

func (re *RethinkDB) GetMenu(id string) (menu models.Menu, err error) {
	res, err := r.DB(re.Database).Table("menus").Get(id).Run(re.session)

	if err != nil {
		return
	}
	defer res.Close()
	err = res.One(&menu)
	return
}

func (re *RethinkDB) DeleteDAG(id string) error {
	_, err := r.DB(re.Database).Table("dags").Get(id).Delete().RunWrite(re.session)
	if err != nil {
		return err
	}
	return nil
}

func (re *RethinkDB) DeleteEdge(id string) error {
	_, err := r.DB(re.Database).Table("edges").Get(id).Delete().RunWrite(re.session)
	if err != nil {
		return err
	}
	return nil
}

func (re *RethinkDB) DeleteNode(id string) error {
	_, err := r.DB(re.Database).Table("nodes").Get(id).Delete().RunWrite(re.session)
	if err != nil {
		return err
	}
	return nil
}

func (re *RethinkDB) DeleteMenu(id string) error {
	_, err := r.DB(re.Database).Table("menus").Get(id).Delete().RunWrite(re.session)
	if err != nil {
		return err
	}
	return nil
}

// func (re *RethinkDB) UpdateDAG(id string, updates map[string]interface{}) error {
func (re *RethinkDB) UpdateDAG(id string, updates models.Dag) error {
	_, err := r.DB(re.Database).Table("dags").Get(id).Update(updates).RunWrite(re.session)
	if err != nil {
		return err
	}

	return nil
}

// func (re *RethinkDB) UpdateDAG(id string, updates map[string]interface{}) error {
func (re *RethinkDB) UpdateMenu(id string, updates models.Menu) error {
	_, err := r.DB(re.Database).Table("menus").Get(id).Update(updates).RunWrite(re.session)
	if err != nil {
		return err
	}

	return nil
}

func (re *RethinkDB) FindRelatedDAG(dag models.Dag) (relatedDAG models.Dag, foundDAG bool, err error) {
	findRelatedDAG := map[string]interface{}{
		"name":        dag.Name,
		"description": dag.Description,
		"diagram":     dag.Diagram,
	}

	relatedDAG, foundDAG, err = re.findOneDAG(findRelatedDAG)

	return
}

func (re *RethinkDB) FindRelatedEdge(edge models.Edge) (relatedEdge models.Edge, foundEdge bool, err error) {
	findRelatedEdge := map[string]interface{}{
		"V":     edge.V,
		"Label": edge.Label,
		"W":     edge.W,
	}

	relatedEdge, foundEdge, err = re.findOneEdge(findRelatedEdge)

	return
}

func (re *RethinkDB) FindRelatedNode(node models.Node) (relatedNode models.Node, foundNode bool, err error) {
	findRelatedNode := map[string]interface{}{
		"V":                    node.V,
		"Parent":               node.Parent,
		"ValueLabel":           node.ValueLabel,
		"ValueType":            node.ValueType,
		"ValueClusterLabelPos": node.ValueClusterLabelPos,
		"ValueStyle":           node.ValueStyle,
	}

	relatedNode, foundNode, err = re.findOneNode(findRelatedNode)

	return
}

func (re *RethinkDB) FindRelatedMenu(menu models.Menu) (relatedMenu models.Menu, foundMenu bool, err error) {
	findRelatedMenu := map[string]interface{}{
		"Parent":  menu.Parent,
		"Options": menu.Options,
	}

	relatedMenu, foundMenu, err = re.findOneMenu(findRelatedMenu)

	return
}

func (re *RethinkDB) findOneDAG(filter interface{}) (dag models.Dag, foundOne bool, err error) {
	dags, err := re.findDAGs(filter)
	if err != nil {
		return
	}
	if len(dags) < 1 {
		return
	}
	if len(dags) >= 1 {
		foundOne = true
		dag = dags[0]
		return
	}
	return
}

func (re *RethinkDB) findOneEdge(filter interface{}) (edge models.Edge, foundOne bool, err error) {
	edges, err := re.findEdges(filter)
	if err != nil {
		return
	}
	if len(edges) < 1 {
		return
	}
	if len(edges) >= 1 {
		foundOne = true
		edge = edges[0]
		return
	}
	return
}

func (re *RethinkDB) findOneNode(filter interface{}) (node models.Node, foundOne bool, err error) {
	nodes, err := re.findNodes(filter)
	if err != nil {
		return
	}
	if len(nodes) < 1 {
		return
	}
	if len(nodes) >= 1 {
		foundOne = true
		node = nodes[0]
		return
	}
	return
}

func (re *RethinkDB) findOneMenu(filter interface{}) (menu models.Menu, foundOne bool, err error) {
	menus, err := re.findMenus(filter)
	if err != nil {
		return
	}
	if len(menus) < 1 {
		return
	}
	if len(menus) >= 1 {
		foundOne = true
		menu = menus[0]
		return
	}
	return
}

func (re *RethinkDB) GetDAGsSummary(queryArgs map[string][]string) (dagsSummary []map[string]interface{}, err error) {
	res, err := r.DB(re.Database).Table("dags").Run(re.session)

	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&dagsSummary)
	if dagsSummary == nil {
		dagsSummary = make([]map[string]interface{}, 0)
		log.Println(dagsSummary)
	}
	return
}

func (re *RethinkDB) GetEdgesSummary(queryArgs map[string][]string) (edgesSummary []map[string]interface{}, err error) {
	res, err := r.DB(re.Database).Table("edges").Run(re.session)

	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&edgesSummary)
	if edgesSummary == nil {
		edgesSummary = make([]map[string]interface{}, 0)
	}
	return
}

func (re *RethinkDB) GetNodesSummary(queryArgs map[string][]string) (edgesSummary []map[string]interface{}, err error) {
	//filter := BuildDAGsFilter(queryArgs)

	res, err := r.DB(re.Database).Table("nodes").Run(re.session)

	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&edgesSummary)
	if edgesSummary == nil {
		edgesSummary = make([]map[string]interface{}, 0)
	}
	return
}

func (re *RethinkDB) GetMenusSummary(queryArgs map[string][]string) (menusSummary []map[string]interface{}, err error) {
	//filter := BuildDAGsFilter(queryArgs)

	res, err := r.DB(re.Database).Table("menus").Run(re.session)

	if err != nil {
		return
	}
	defer res.Close()
	err = res.All(&menusSummary)
	if menusSummary == nil {
		menusSummary = make([]map[string]interface{}, 0)
	}
	return
}

func (re *RethinkDB) GetMenusOptions(queryArgs map[string][]string) (menusOptions map[string]models.Menu, err error) {
	res, err := r.DB(re.Database).Table("menus").Run(re.session)

	if err != nil {
		return
	}
	defer res.Close()
	var menu []models.Menu
	err = res.All(&menu)
	data := map[string]models.Menu{}
	if menusOptions == nil {
		for _, m := range menu {
			//fmt.Println(m.Id)
			data[m.Id] = m
		}
		menusOptions = data
	}
	//log.Println("menusOptions")
	//log.Println(menusOptions)

	return
}

// ─── Shares ───────────────────────────────────────────────────────────────────

func (re *RethinkDB) InitSharesTables() error {
	for _, table := range []string{"shares", "share_denylist"} {
		if err := re.createTableIfNotExist(table); err != nil {
			return err
		}
	}
	return re.createIndexIfNotExist("shares", "dag_id")
}

func (re *RethinkDB) CreateShare(s models.Share) error {
	_, err := r.DB(re.Database).Table("shares").Insert(s).RunWrite(re.session)
	return err
}

func (re *RethinkDB) RevokeShare(jti string) error {
	entry := models.ShareDenylist{Jti: jti, RevokedAt: time.Now()}
	_, err := r.DB(re.Database).Table("share_denylist").Insert(entry, r.InsertOpts{Conflict: "replace"}).RunWrite(re.session)
	return err
}

func (re *RethinkDB) IsRevoked(jti string) (bool, error) {
	res, err := r.DB(re.Database).Table("share_denylist").Get(jti).Run(re.session)
	if err != nil {
		return false, err
	}
	defer res.Close()
	return !res.IsNil(), nil
}

// ─── Users (local auth) ───────────────────────────────────────────────────────

func (re *RethinkDB) InitUsersTable() error {
	if err := re.createTableIfNotExist("users"); err != nil {
		return err
	}
	return re.createIndexIfNotExist("users", "username")
}

func (re *RethinkDB) CreateUser(u models.User) error {
	_, err := r.DB(re.Database).Table("users").Insert(u).RunWrite(re.session)
	return err
}

func (re *RethinkDB) GetUser(username string) (models.User, error) {
	res, err := r.DB(re.Database).Table("users").
		GetAllByIndex("username", username).
		Limit(1).
		Run(re.session)
	if err != nil {
		return models.User{}, err
	}
	defer res.Close()
	var u models.User
	res.One(&u)
	return u, nil
}

// ─── Index helpers ────────────────────────────────────────────────────────────

func (re *RethinkDB) createIndexIfNotExist(table, index string) error {
	res, err := r.DB(re.Database).Table(table).IndexList().Run(re.session)
	if err != nil {
		return err
	}
	defer res.Close()
	var indexes []string
	if err = res.All(&indexes); err != nil {
		return err
	}
	for _, idx := range indexes {
		if idx == index {
			return nil
		}
	}
	_, err = r.DB(re.Database).Table(table).IndexCreate(index).RunWrite(re.session)
	if err != nil {
		return err
	}
	_, err = r.DB(re.Database).Table(table).IndexWait(index).Run(re.session)
	return err
}

// ─── History ──────────────────────────────────────────────────────────────────

const historyLimit = 50

func (re *RethinkDB) AppendHistory(dagId, snapshotJSON, savedBy string) error {
	entry := models.DagHistory{
		Id:           uuid.New().String(),
		DagId:        dagId,
		SnapshotJSON: snapshotJSON,
		SavedBy:      savedBy,
		SavedAt:      time.Now(),
	}
	_, err := r.DB(re.Database).Table("dag_history").Insert(entry).RunWrite(re.session)
	if err != nil {
		return err
	}
	go re.pruneHistory(dagId)
	return nil
}

func (re *RethinkDB) pruneHistory(dagId string) {
	res, err := r.DB(re.Database).Table("dag_history").
		GetAllByIndex("dag_id", dagId).
		OrderBy(r.Asc("saved_at")).
		Field("id").
		Run(re.session)
	if err != nil {
		return
	}
	defer res.Close()
	var ids []string
	if err = res.All(&ids); err != nil || len(ids) <= historyLimit {
		return
	}
	toDelete := ids[:len(ids)-historyLimit]
	for _, id := range toDelete {
		r.DB(re.Database).Table("dag_history").Get(id).Delete().RunWrite(re.session) //nolint:errcheck
	}
}

func (re *RethinkDB) GetHistory(dagId string) ([]models.DagHistory, error) {
	res, err := r.DB(re.Database).Table("dag_history").
		GetAllByIndex("dag_id", dagId).
		OrderBy(r.Desc("saved_at")).
		Limit(historyLimit).
		Run(re.session)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	var history []models.DagHistory
	err = res.All(&history)
	if history == nil {
		history = []models.DagHistory{}
	}
	return history, err
}

func (re *RethinkDB) RestoreHistory(historyId, dagId string) error {
	res, err := r.DB(re.Database).Table("dag_history").Get(historyId).Run(re.session)
	if err != nil {
		return err
	}
	defer res.Close()
	var entry models.DagHistory
	if err = res.One(&entry); err != nil {
		return err
	}
	restore := models.Dag{
		Id:      dagId,
		Diagram: entry.SnapshotJSON,
	}
	return re.UpdateDAG(dagId, restore)
}


