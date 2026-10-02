//go:build js && wasm

// Browser build: exposes the formatter to the static site (no Discord access).
package main

import (
	"encoding/json"
	"errors"
	"syscall/js"
	"time"
)

func main() {
	js.Global().Set("dombotFormat", js.FuncOf(func(_ js.Value, args []js.Value) any {
		res, err := process(args[0].String(), args[1].String(), false, 0)
		if err != nil {
			return toJSON(map[string]string{"error": err.Error()})
		}
		return toJSON(res)
	}))
	js.Global().Set("dombotSample", js.FuncOf(func(js.Value, []js.Value) any {
		return toJSON(map[string]string{"chat": sampleChat, "dombot": sampleDomBot})
	}))
	select {}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func askDomBot(string, time.Duration) ([]message, error) {
	return nil, errors.New("asking DomBot needs the local app (dombot web)")
}
