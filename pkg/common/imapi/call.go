package imapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"chat/pkg/common/constant"
	"chat/pkg/common/errs"
	"chat/pkg/common/log"
	constantpb "github.com/openimsdk/protocol/constant"
	"gorm.io/gorm/utils"
)

type baseApiResponse[T any] struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	ErrDlt  string `json:"errDlt"`
	Data    *T     `json:"data"`
}

var sharedTransport = &http.Transport{
	MaxIdleConns:        200,
	MaxIdleConnsPerHost: 100,
	IdleConnTimeout:     90 * time.Second,
}

var client = &http.Client{
	Timeout:   time.Second * 10,
	Transport: sharedTransport,
}

var longClient = &http.Client{
	Timeout:   10 * time.Minute,
	Transport: sharedTransport,
}

type ApiCaller[Req, Resp any] interface {
	Call(ctx context.Context, apiPrefix string, req *Req) (*Resp, error)
	CallWithQuery(ctx context.Context, apiPrefix string, req *Req, queryParams map[string]string) (*Resp, error)
}

func NewApiCaller[Req, Resp any](api string) ApiCaller[Req, Resp] {
	return &caller[Req, Resp]{
		api:    api,
		client: client,
	}
}

func NewLongApiCaller[Req, Resp any](api string) ApiCaller[Req, Resp] {
	return &caller[Req, Resp]{
		api:    api,
		client: longClient,
	}
}

type caller[Req, Resp any] struct {
	api    string
	client *http.Client
}

func (a caller[Req, Resp]) httpClient() *http.Client {
	if a.client != nil {
		return a.client
	}
	return client
}

func (a caller[Req, Resp]) Call(ctx context.Context, apiPrefix string, req *Req) (*Resp, error) {
	start := time.Now()
	resp, err := a.call(ctx, apiPrefix, req)
	if err != nil {
		if resp != nil {
			log.ZError(ctx, "api caller failed", err, "api", a.api, "duration", time.Since(start), "req", req, "resp", resp)
		} else {
			log.ZError(ctx, "api caller failed", err, "api", a.api, "duration", time.Since(start), "req", req)
		}
		return nil, err
	}
	log.ZInfo(ctx, "api caller success resp", "api", a.api, "duration", time.Since(start), "req", req, "resp", resp)
	return resp, nil
}

func (a caller[Req, Resp]) call(ctx context.Context, apiPrefix string, req *Req) (*Resp, error) {
	u := apiPrefix + a.api
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	operationID := utils.ToString(ctx.Value(constantpb.OperationID))
	request.Header.Set(constantpb.OperationID, operationID)
	if token, _ := ctx.Value(constant.CtxApiToken).(string); token != "" {
		request.Header.Set(constantpb.Token, token)
	}
	response, err := a.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.ZError(ctx, "", err)
		}
	}(response.Body)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errs.WrapMsg(err, "read http response body", "url", u, "code", response.StatusCode)
	}
	var resp baseApiResponse[Resp]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, errs.WrapMsg(err, string(data))
	}
	if resp.ErrCode != 0 {
		return nil, errs.NewCodeError(resp.ErrCode, resp.ErrMsg).WithDetail(resp.ErrDlt).Wrap()
	}
	return resp.Data, nil
}

func (a caller[Req, Resp]) CallWithQuery(ctx context.Context, apiPrefix string, req *Req, queryParams map[string]string) (*Resp, error) {
	start := time.Now()
	resp, err := a.callWithQuery(ctx, apiPrefix, req, queryParams)
	if err != nil {
		if resp != nil {
			log.ZError(ctx, "api caller failed", err, "api", a.api, "duration", time.Since(start), "req", req, "resp", resp)
		} else {
			log.ZError(ctx, "api caller failed", err, "api", a.api, "duration", time.Since(start), "req", req)
		}
		return nil, err
	}
	log.ZInfo(ctx, "api caller success resp", "api", a.api, "duration", time.Since(start), "req", req, "resp", resp)
	return resp, nil
}

func (a caller[Req, Resp]) callWithQuery(ctx context.Context, apiPrefix string, req *Req, queryParams map[string]string) (*Resp, error) {
	fullURL := apiPrefix + a.api
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, errs.WrapMsg(err, "failed to parse URL", fullURL)
	}

	query := parsedURL.Query()

	for key, value := range queryParams {
		query.Set(key, value)
	}

	parsedURL.RawQuery = query.Encode()
	fullURL = parsedURL.String()
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	operationID := utils.ToString(ctx.Value(constantpb.OperationID))
	request.Header.Set(constantpb.OperationID, operationID)
	if token, _ := ctx.Value(constant.CtxApiToken).(string); token != "" {
		request.Header.Set(constantpb.Token, token)
	}
	response, err := a.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.ZError(ctx, "", err)
		}
	}(response.Body)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errs.WrapMsg(err, "read http response body", "fullUrl", fullURL, "code", response.StatusCode)
	}
	var resp baseApiResponse[Resp]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, errs.WrapMsg(err, string(data))
	}
	if resp.ErrCode != 0 {
		return nil, errs.NewCodeError(resp.ErrCode, resp.ErrMsg).WithDetail(resp.ErrDlt).Wrap()
	}
	return resp.Data, nil
}
