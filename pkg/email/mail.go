package email

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"gopkg.in/gomail.v2"
	"html/template"
	"time"
)

//go:embed templates/verify_code.html
var verifyCodeTpl string

var verifyCodeTemplate = template.Must(template.New("verify_code").Parse(verifyCodeTpl))

const (
	defaultAppName       = "KurbanIM"
	defaultExpireMinutes = 5
	defaultScene         = "身份验证"
)

type Mail interface {
	Name() string
	SendMail(ctx context.Context, mail string, verifyCode string) error
}

func NewMail(smtpAddr string, smtpPort int, senderMail, senderAuthorizationCode, title string) Mail {
	dail := gomail.NewDialer(smtpAddr, smtpPort, senderMail, senderAuthorizationCode)
	return &mail{
		title:      title,
		senderMail: senderMail,
		dail:       dail,
	}
}

type mail struct {
	senderMail string
	title      string
	dail       *gomail.Dialer
}

func (m *mail) Name() string {
	return "mail"
}

type verifyCodeData struct {
	AppName       string
	Code          string
	Scene         string
	ExpireMinutes int
	Year          int
}

func (m *mail) renderVerifyCode(verifyCode string) (string, error) {
	data := verifyCodeData{
		AppName:       defaultAppName,
		Code:          verifyCode,
		Scene:         defaultScene,
		ExpireMinutes: defaultExpireMinutes,
		Year:          time.Now().Year(),
	}
	var buf bytes.Buffer
	if err := verifyCodeTemplate.Execute(&buf, data); err != nil {
		return "", errs.Wrap(err)
	}
	return buf.String(), nil
}

func (m *mail) SendMail(ctx context.Context, to string, verifyCode string) error {
	body, err := m.renderVerifyCode(verifyCode)
	if err != nil {
		body = fmt.Sprintf("Your verification code is: %s. This code is valid for %d minutes and should not be shared with others",
			verifyCode, defaultExpireMinutes)
	}

	msg := gomail.NewMessage()
	msg.SetHeader(`From`, m.senderMail)
	msg.SetHeader(`To`, []string{to}...)
	msg.SetHeader(`Subject`, m.title)
	msg.SetBody(`text/html`, body)
	return errs.Wrap(m.dail.DialAndSend(msg))
}
