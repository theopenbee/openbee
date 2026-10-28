package worker

import "github.com/theopenbee/openbee/desktop/internal/api"

func (p *Panel) SetOnDelete(fn func(w api.Worker)) {
	p.onDelete = fn
}
