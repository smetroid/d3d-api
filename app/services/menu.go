package services

import (
	"log"

	"github.com/smetroid/d3d-api/app/db/rethinkdb"
	"github.com/smetroid/d3d-api/app/models"
)

type MenuService struct {
	DB *rethinkdb.RethinkDB
}

func (ds *MenuService) GetMenu(id string) (MenuResponse models.MenuResponse, err error) {
	Menu, err := ds.DB.GetMenu(id)
	MenuResponse = models.NewMenuResponse(Menu)

	return
}

func (ds *MenuService) GetMenus(queryParams map[string][]string) (MenusResponse models.MenusResponse, err error) {
	Menus, err := ds.DB.GetMenusSummary(queryParams)
	if err != nil {
		return
	}
	MenusResponse = models.NewMenusResponse(Menus)

	return
}

func (ds *MenuService) GetMenusOptions(queryParams map[string][]string) (MenusOptionsResponse models.MenusOptionsResponse, err error) {
	Menus, err := ds.DB.GetMenusOptions(queryParams)
	if err != nil {
		return
	}
	MenusOptionsResponse = models.NewMenusOptionsResponse(Menus)

	return
}

func (ds *MenuService) DeleteMenu(id string) (err error) {
	err = ds.DB.DeleteMenu(id)
	return
}

func (ds *MenuService) ProcessMenu(currentMenu models.Menu) (id string, err error) {
	currentMenu.GenerateDefaults()
	existingMenu, foundExistingMenu, err := ds.DB.FindRelatedMenu(currentMenu)

	if !foundExistingMenu {
		//new Menu
		id, err = ds.DB.CreateMenu(currentMenu)
		if err != nil {
			log.Println(err)
		}
		return
	}

	log.Println(existingMenu)
	log.Println("menus")

	return
}

func (ds *MenuService) UpdateMenu(id string, menu models.Menu) (err error) {
	err = ds.DB.UpdateMenu(id, menu)
	return
}
