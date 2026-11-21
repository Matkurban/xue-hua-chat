package chat

import "chat/pkg/common/db/pagination"

type simplePage struct {
	pageNumber int32
	showNumber int32
}

func (p simplePage) GetPageNumber() int32 { return p.pageNumber }
func (p simplePage) GetShowNumber() int32 { return p.showNumber }

// newPage 从 pageNumber/showNumber 构造 pagination.Pagination
func newPage(pageNumber, showNumber int32) pagination.Pagination {
	return simplePage{pageNumber: pageNumber, showNumber: showNumber}

}
