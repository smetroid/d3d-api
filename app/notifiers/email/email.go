package email

import (
	"crypto/tls"
	"log"

	"github.com/smetroid/d3d-api/app/models"
	"gopkg.in/gomail.v2"
)

type Email struct {
	Addresses     []string `toml:"email"`
	EnabledField  bool     `toml:"enabled"`
	SmtpServer    string   `toml:"smtp_server"`
	SmtpPort      int      `toml:"smtp_port"`
	SmtpUser      string   `toml:"smtp_user"`
	SmtpPassword  string   `toml:"smtp_password"`
	SkipSslVerify bool     `toml:"skip_ssl_verify"`
	From          string   `toml:"from"`
	SamusUrl      string   `toml:"samus_url"`
}

func (em *Email) Init() error {
	return nil
}

func (em *Email) Enabled() bool {
	return em.EnabledField
}

func (em *Email) CreateEmailEvent(eventType string, alert models.Dag) error {
	d := gomail.NewDialer(em.SmtpServer, em.SmtpPort, em.SmtpUser, em.SmtpPassword)
	if em.SkipSslVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: em.SkipSslVerify}
	}
	s, err := d.Dial()
	if err != nil {
		return (err)
	}

	m := gomail.NewMessage()

	for _, mail := range em.Addresses {
		m.SetHeader("From", em.From)
		m.SetHeader("To", mail)
		m.SetHeader("DAG")
		m.SetBody("text/plain", "DAG Entry URL:\n")

		if err := gomail.Send(s, m); err != nil {
			log.Printf("Could not send mail to %q: %v", mail, err)
		}
		m.Reset()
	}

	return (err)
}
