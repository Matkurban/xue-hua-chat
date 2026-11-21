package sms

import (
	"context"
	"encoding/json"

	"chat/pkg/common/errs"
	aliconf "github.com/alibabacloud-go/darabonba-openapi/client"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v2/client"
)

func NewAli(endpoint, accessKeyId, accessKeySecret, signName, verificationCodeTemplateCode string) (SMS, error) {
	conf := &aliconf.Config{
		Endpoint:        new(endpoint),
		AccessKeyId:     new(accessKeyId),
		AccessKeySecret: new(accessKeySecret),
	}
	client, err := dysmsapi.NewClient(conf)
	if err != nil {
		return nil, err
	}
	return &ali{
		signName:                     signName,
		verificationCodeTemplateCode: verificationCodeTemplateCode,
		client:                       client,
	}, nil
}

type ali struct {
	signName                     string
	verificationCodeTemplateCode string
	client                       *dysmsapi.Client
}

func (a *ali) Name() string {
	return "ali-sms"
}

func (a *ali) SendCode(ctx context.Context, areaCode string, phoneNumber string, verifyCode string) error {
	data, err := json.Marshal(&struct {
		Code string `json:"code"`
	}{Code: verifyCode})
	if err != nil {
		return errs.Wrap(err)
	}
	req := &dysmsapi.SendSmsRequest{
		PhoneNumbers:  new(areaCode + phoneNumber),
		SignName:      new(a.signName),
		TemplateCode:  new(a.verificationCodeTemplateCode),
		TemplateParam: new(string(data)),
	}
	_, err = a.client.SendSms(req)
	return errs.Wrap(err)
}
