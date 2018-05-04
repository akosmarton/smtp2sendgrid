package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strings"
	"unicode"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
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

func Add(m *sgmail.SGMailV3, contentType string, r io.Reader) error {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}

	fmt.Println(contentType)

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(r, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			if err := Add(m, p.Header.Get("Content-Type"), p); err != nil {
				return err
			}
		}
	} else if strings.HasPrefix(mediaType, "text/") {
		b, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		m.AddContent(sgmail.NewContent(mediaType, string(b)))
	} else {
		a := sgmail.NewAttachment()
		a.SetType(mediaType)
		if params["name"] == "" {
			a.SetFilename(uuid.New().String())
		} else {
			a.SetFilename(params["name"])
		}

		bi := bytes.Buffer{}
		if _, err := bi.ReadFrom(r); err != nil {
			return err
		}

		if _, err := base64.StdEncoding.DecodeString(bi.String()); err == nil {
			a.SetContent(stripSpaces(bi.String()))
		} else {
			a.SetContent(base64.StdEncoding.EncodeToString(bi.Bytes()))
		}
		m.AddAttachment(a)
	}

	return nil
}

func (u *User) Send(_ string, _ []string, r io.Reader) error {
	mi, err := mail.ReadMessage(r)
	if err != nil {
		return err
	}

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

	mo.AddPersonalizations(p)

	if err := Add(mo, mi.Header.Get("Content-Type"), mi.Body); err != nil {
		return err
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

func stripSpaces(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			// if the character is a space, drop it
			return -1
		}
		// else keep it in the string
		return r
	}, str)
}
