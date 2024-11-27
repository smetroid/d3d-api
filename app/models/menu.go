package models

import (
	"time"

	"github.com/google/uuid"
)

type Menu struct {
	//globally unique random UUID
	Id string `gorethink:"id,opmitempty" json:"id"`
	//UTC date and time the alert was generated in ISO 8601 format
	Created time.Time `gorethink:"created" json:"created"`
	Name    string    `gorethink:"name" json:"name"`
	Parent  string    `gorethink:"parent" json:"parent"`
	Options string    `gorethink:"options" json:"options"`
}

type MenuResponse struct {
	Status      string    `json:"status"`
	LastTime    time.Time `json:"lastTime"`
	AutoRefresh bool      `json:"autoRefresh"`
	Name        string    `json:"name"`
	Options     string    `json:"options"`
	Total       int       `json:"total"`
}

type MenusResponse struct {
	Status      string                   `json:"status"`
	Menus       []map[string]interface{} `json:"menus"`
	LastTime    time.Time                `json:"lastTime"`
	AutoRefresh bool                     `json:"autoRefresh"`
	Total       int                      `json:"total"`
}

type MenusOptionsResponse struct {
	Status      string          `json:"status"`
	Menus       map[string]Menu `json:"menus"`
	LastTime    time.Time       `json:"lastTime"`
	AutoRefresh bool            `json:"autoRefresh"`
	Total       int             `json:"total"`
}

func NewMenuResponse(menu Menu) (mr MenuResponse) {
	mr = MenuResponse{}
	mr.Status = "ok"
	mr.AutoRefresh = true
	mr.Name = menu.Name
	mr.Options = menu.Options
	return
}

func NewMenusResponse(menus []map[string]interface{}) (mr MenusResponse) {
	mr = MenusResponse{}
	mr.Menus = menus
	mr.Status = "ok"
	mr.AutoRefresh = false
	mr.Total = len(menus)
	return
}

func NewMenusOptionsResponse(menus map[string]Menu) (mr MenusOptionsResponse) {
	mr = MenusOptionsResponse{}
	mr.Menus = menus
	mr.Status = "ok"
	mr.AutoRefresh = false
	mr.Total = len(menus)
	return
}

func (menu *Menu) GenerateDefaults() {
	if menu.Id == "" {
		id := uuid.Must(uuid.NewRandom())
		menu.Id = id.String()
	}

	if menu.Created.IsZero() {
		menu.Created = time.Now()
	}
}
