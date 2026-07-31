package a

import "context"

func main() {
	ctx := context.Background() // ok: main() is exempt
	_ = ctx
}

func handleRequest() {
	ctx := context.Background() // want "context.Background\\(\\) used outside main\\(\\)"
	_ = ctx
}

func handleRequestWithParent(parent context.Context) {
	_ = parent // ok: propagates caller's context, no Background() call
}
