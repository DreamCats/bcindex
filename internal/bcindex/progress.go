package bcindex

import (
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

type ProgressReporter interface {
	Start(total int)
	Increment()
	Finish()
}

type IndexProgress struct {
	enabled bool
	bar     *progressbar.ProgressBar
}

func NewIndexProgress(enabled bool) ProgressReporter {
	if !enabled {
		return nil
	}
	return &IndexProgress{enabled: true}
}

func (p *IndexProgress) Start(total int) {
	if !p.enabled || total <= 0 {
		return
	}
	p.bar = progressbar.NewOptions(total,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetDescription("indexing"),
		progressbar.OptionSetWidth(32),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
}

func (p *IndexProgress) Increment() {
	if p.bar == nil {
		return
	}
	_ = p.bar.Add(1)
}

func (p *IndexProgress) Finish() {
	if p.bar == nil {
		return
	}
	_ = p.bar.Finish()
}

func DefaultProgressEnabled() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func StartSpinner(enabled bool, desc string) func() {
	if !enabled {
		return func() {}
	}
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSpinnerType(9),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionSetWidth(10),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = bar.Add(1)
			case <-done:
				_ = bar.Finish()
				return
			}
		}
	}()
	return func() { close(done) }
}
