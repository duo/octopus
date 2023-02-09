package manager

import "math"

type Pager struct {
	NumPages           int
	HasPrev, HasNext   bool
	PrevPage, NextPage int
	ItemsPerPage       int
	CurrentPage        int
	NumItems           int
}

func CalcPager(currentPage, itemsPerPage, numItems int) Pager {
	p := Pager{}

	p.NumItems = numItems
	p.ItemsPerPage = itemsPerPage

	p.NumPages = int(math.Ceil(float64(p.NumItems) / float64(p.ItemsPerPage)))

	if currentPage <= 0 {
		p.CurrentPage = 1
	} else if currentPage > p.NumPages {
		p.CurrentPage = p.NumPages
	} else {
		p.CurrentPage = currentPage
	}

	p.HasPrev = p.CurrentPage > 1
	p.HasNext = p.CurrentPage < p.NumPages

	if p.HasPrev {
		p.PrevPage = p.CurrentPage - 1
	}
	if p.HasNext {
		p.NextPage = p.CurrentPage + 1
	}

	return p
}
