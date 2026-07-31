package legacypkg

import "context"

func doWork() {
	ctx := context.Background() // ok: package path matches -allow=legacypkg
	_ = ctx
}
