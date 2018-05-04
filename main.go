package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strings"

	"github.com/emersion/go-smtp"
	sendgrid "github.com/sendgrid/sendgrid-go"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

type Backend struct{}

func (bkd *Backend) Login(username, password string) (smtp.User, error) {
	return &User{}, nil
}

func (bkd *Backend) AnonymousLogin() (smtp.User, error) {
	return &User{}, nil
}

type User struct{}

func (u *User) Send(_ string, _ []string, r io.Reader) error {
	mi, err := mail.ReadMessage(r)

	mo := new(sgmail.SGMailV3)

	p := sgmail.NewPersonalization()
	from, _ := mi.Header.AddressList("From")
	to, _ := mi.Header.AddressList("To")
	cc, _ := mi.Header.AddressList("Cc")
	bcc, _ := mi.Header.AddressList("Bcc")
	replyto, _ := mi.Header.AddressList("Reply-To")

	for _, v := range from {
		mo.SetFrom(sgmail.NewEmail(v.Name, v.Address))
	}
	for _, v := range to {
		p.AddTos(sgmail.NewEmail(v.Name, v.Address))
	}
	for _, v := range cc {
		p.AddCCs(sgmail.NewEmail(v.Name, v.Address))
	}
	for _, v := range bcc {
		p.AddBCCs(sgmail.NewEmail(v.Name, v.Address))
	}
	for _, v := range replyto {
		mo.SetReplyTo(sgmail.NewEmail(v.Name, v.Address))
	}

	mo.Subject = mi.Header.Get("Subject")
	mo.SetHeader("Date", mi.Header.Get("Date"))

	mediaType, params, err := mime.ParseMediaType(mi.Header.Get("Content-Type"))
	if err != nil {
		log.Println(err)
		return err
	}

	mo.AddPersonalizations(p)

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(mi.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			mediaType, _, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
			if err != nil {
				log.Println(err)
				return err
			}
			if strings.HasPrefix(mediaType, "multipart/") {
				slurp, err := ioutil.ReadAll(p)
				if err != nil {
					return err
				}
				mo.AddContent(sgmail.NewContent(mediaType, string(slurp)))
			} else if strings.HasPrefix(mediaType, "text/") {
				slurp, err := ioutil.ReadAll(p)
				if err != nil {
					return err
				}
				mo.AddContent(sgmail.NewContent(mediaType, string(slurp)))
			} else {
				b := bytes.Buffer{}
				_, err := b.ReadFrom(base64.NewDecoder(base64.StdEncoding, p))
				if err != nil {
					return err
				}
				s := base64.StdEncoding.EncodeToString(b.Bytes())
				a := sgmail.NewAttachment()
				a.SetContent(s)
				a.SetType(mediaType)
				a.SetFilename(p.FileName())
				mo.AddAttachment(a)
			}
		}
	} else {
		body, err := ioutil.ReadAll(mi.Body)
		if err != nil {
			return err
		}
		mo.AddContent(sgmail.NewContent(mediaType, string(body)))
	}

	response, err := client.Send(mo)
	if err != nil {
		log.Println(err)
		return err
	} else {
		if response.StatusCode < 300 {
			log.Println("Message sent successfully to", to, response.StatusCode)
		} else {
			log.Println("Message failed to send to", to, response.StatusCode)
			return errors.New(response.Body)
		}
	}

	return nil
}

func (u *User) Logout() error {
	return nil
}

var client *sendgrid.Client

func main() {
	be := &Backend{}

	client = sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))

	s := smtp.NewServer(be)

	s.Addr = os.Getenv("LISTEN_ADDR")
	s.Domain = os.Getenv("DOMAIN")
	s.MaxIdleSeconds = 300
	s.MaxMessageBytes = 1024 * 1024 * 32
	s.MaxRecipients = 50
	s.AuthDisabled = true

	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
