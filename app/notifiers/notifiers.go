package notifiers

import (
	"log"

	"github.com/smetroid/d3d-api/app/notifiers/email"
	"github.com/smetroid/d3d-api/app/notifiers/file"
)

type Notifiers struct {
	File      file.File   `toml:"file"`
	Email     email.Email `toml:"email"`
	notifiers []Notifier
}

type Notifier interface {
	Init() error
	Enabled() bool
}

func (ns *Notifiers) Init() {
	uninitializedNotifiers := []Notifier{&ns.File, &ns.Email}

	for _, notifier := range uninitializedNotifiers {
		if notifier.Enabled() {
			err := notifier.Init()
			if err != nil {
				log.Println(err)
			} else {
				ns.notifiers = append(ns.notifiers, notifier)
			}
		}
	}
}
