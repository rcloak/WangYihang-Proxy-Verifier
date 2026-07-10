package processer

import (
	"time"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
)

func Monitor(interval int, state *model.State) {
	for {
		// Sleep for interval seconds
		time.Sleep(time.Duration(interval) * time.Second)
		// Take a snapshot
		state.Snapshot()
	}
}
