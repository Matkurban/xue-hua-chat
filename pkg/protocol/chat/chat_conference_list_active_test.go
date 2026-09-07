package chat

import "testing"

func TestListActiveConferencesReq_NormalizePage(t *testing.T) {
	req := &ListActiveConferencesReq{}
	page, size := req.NormalizePage()
	if page != 1 || size != 50 {
		t.Fatalf("defaults page=%d size=%d", page, size)
	}
	req.PageNumber = 2
	req.ShowNumber = 10
	page, size = req.NormalizePage()
	if page != 2 || size != 10 {
		t.Fatalf("explicit page=%d size=%d", page, size)
	}
	req.ShowNumber = 500
	_, size = req.NormalizePage()
	if size != 100 {
		t.Fatalf("cap size=%d", size)
	}
	req.Pagination = &struct {
		PageNumber int32 `json:"pageNumber"`
		ShowNumber int32 `json:"showNumber"`
	}{PageNumber: 3, ShowNumber: 20}
	page, size = req.NormalizePage()
	if page != 3 || size != 20 {
		t.Fatalf("pagination nested page=%d size=%d", page, size)
	}
}
