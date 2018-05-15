package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"os"
	"strings"
	"unicode"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func Add(m *sgmail.SGMailV3, h textproto.MIMEHeader, r io.Reader) error {
	mediaType, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		return err
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(r, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			if err := Add(m, p.Header, p); err != nil {
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
		if params["name"] != "" {
			a.SetFilename(params["name"])
		} else {
			a.SetFilename(uuid.New().String())
		}
		cid := h.Get("Content-Id")
		if cid != "" {
			cid = strings.TrimPrefix(cid, "<")
			cid = strings.TrimSuffix(cid, ">")
			a.SetContentID(cid)
			if strings.HasPrefix(h.Get("Content-Disposition"), "inline;") {
				a.SetDisposition("inline")
			}
		}

		bi := bytes.Buffer{}
		if _, err := bi.ReadFrom(r); err != nil {
			return err
		}

		if h.Get("Content-Transfer-Encoding") == "base64" {
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

	h := make(textproto.MIMEHeader)

	h.Set("Content-Type", mi.Header.Get("Content-Type"))

	if err := Add(mo, h, mi.Body); err != nil {
		return err
	}

	response, err := client.Send(mo)
	if err != nil {
		log.Error().Msg(err.Error())
		return err
	} else {
		if response.StatusCode < 300 {
			log.Info().Str("to", to[0].String()).Int("statusCode", response.StatusCode).Msg("Message sent successfull")
		} else {
			log.Error().Str("to", to[0].String()).Int("statusCode", response.StatusCode).Msg("Message failed to send")
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
	log.Logger = log.Hook(SeverityHook{})

	be := &Backend{}

	client = sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))

	s := smtp.NewServer(be)

	s.Addr = os.Getenv("LISTEN_ADDR")
	s.Domain = os.Getenv("DOMAIN")
	s.MaxIdleSeconds = 300
	s.MaxMessageBytes = 1024 * 1024 * 32
	s.MaxRecipients = 50
	s.AuthDisabled = true

	log.Info().Str("addr", s.Addr).Msg("Starting server")
	if err := s.ListenAndServe(); err != nil {
		log.Fatal().Msg(err.Error())
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

type SeverityHook struct{}

func (h SeverityHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level != zerolog.NoLevel {
		e.Str("severity", level.String())
	}
}
