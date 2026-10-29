package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"time"

	"github.com/dop251/goja"
)

const (
	maxOutput      = 60000
	maxCallDepth   = 2048
	maxGoStack     = 64 << 20
	memoryPollGap  = 20 * time.Millisecond
	defaultMemory  = 512 << 20
	heapObjectsKey = "/memory/classes/heap/objects:bytes"
)

var memoryLimit uint64 = defaultMemory

var (
	errPending   = errors.New("the script awaited a promise that never settled")
	errMemory    = errors.New("the script used too much memory")
	errCallDepth = errors.New("the script exceeded the maximum call stack size")
)

func MaybeRunChild() {
	if os.Getenv(childEnv) != "1" {
		return
	}
	debug.SetMaxStack(maxGoStack)
	if err := serveChild(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func serveChild(in io.Reader, out io.Writer) error {
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)
	var req request
	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("read script: %w", err)
	}
	if req.MemoryLimit > 0 {
		memoryLimit = req.MemoryLimit
	}
	debug.SetMemoryLimit(int64(memoryLimit))
	call := func(name string, args json.RawMessage) (json.RawMessage, error) {
		if err := enc.Encode(childMessage{Call: &childCall{Name: name, Args: args}}); err != nil {
			return nil, err
		}
		var rep reply
		if err := dec.Decode(&rep); err != nil {
			return nil, err
		}
		if rep.Error != "" {
			return nil, errors.New(rep.Error)
		}
		return rep.Result, nil
	}
	output, err := execute(req.Code, req.Methods, req.Timeout, call)
	msg := childMessage{Done: true, Output: output}
	if err != nil {
		msg.Error = err.Error()
	}
	return enc.Encode(msg)
}

type scriptRun struct {
	vm        *goja.Runtime
	stringify goja.Callable
	parse     goja.Callable
	logs      []string
}

func execute(code string, methods []string, timeout time.Duration, call func(string, json.RawMessage) (json.RawMessage, error)) (output string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r, err := newScriptRun()
	if err != nil {
		return "", err
	}
	defer func() {
		if p := recover(); p != nil {
			output, err = r.output(""), fmt.Errorf("%s: %v", crashedError, p)
		}
	}()

	api := r.vm.NewObject()
	for _, name := range methods {
		if err := api.Set(name, func(fc goja.FunctionCall) goja.Value {
			return r.call(name, fc.Argument(0), call)
		}); err != nil {
			return "", err
		}
	}
	if err := r.vm.Set("openbee", api); err != nil {
		return "", err
	}

	stop := context.AfterFunc(ctx, func() { r.vm.Interrupt(ctx.Err()) })
	defer stop()
	go r.watchMemory(ctx)

	v, err := r.vm.RunString("(async () => {" + code + "\n})()")
	if err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return r.output(""), fmt.Errorf("the script timed out after %s", timeout)
			}
			if cause, ok := interrupted.Value().(error); ok {
				return r.output(""), cause
			}
		}
		var overflow *goja.StackOverflowError
		if errors.As(err, &overflow) {
			return r.output(""), errCallDepth
		}
		return r.output(""), err
	}
	p, ok := v.Export().(*goja.Promise)
	if !ok {
		return r.output(r.format(v)), nil
	}
	switch p.State() {
	case goja.PromiseStateFulfilled:
		return r.output(r.format(p.Result())), nil
	case goja.PromiseStateRejected:
		return r.output(""), errors.New(r.describeError(p.Result()))
	default:
		return r.output(""), errPending
	}
}

func newScriptRun() (*scriptRun, error) {
	vm := goja.New()
	vm.SetMaxCallStackSize(maxCallDepth)
	jsonObj := vm.Get("JSON").ToObject(vm)
	stringify, ok := goja.AssertFunction(jsonObj.Get("stringify"))
	if !ok {
		return nil, errors.New("JSON.stringify is not available")
	}
	parse, ok := goja.AssertFunction(jsonObj.Get("parse"))
	if !ok {
		return nil, errors.New("JSON.parse is not available")
	}
	r := &scriptRun{vm: vm, stringify: stringify, parse: parse}

	console := vm.NewObject()
	logFn := func(fc goja.FunctionCall) goja.Value {
		parts := make([]string, len(fc.Arguments))
		for i, a := range fc.Arguments {
			parts[i] = r.format(a)
		}
		r.logs = append(r.logs, strings.Join(parts, " "))
		return goja.Undefined()
	}
	for _, name := range []string{"log", "info", "warn", "error", "debug"} {
		if err := console.Set(name, logFn); err != nil {
			return nil, err
		}
	}
	if err := vm.Set("console", console); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *scriptRun) watchMemory(ctx context.Context) {
	base := heapBytes()
	ticker := time.NewTicker(memoryPollGap)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if used := heapBytes(); used > base && used-base > memoryLimit {
				r.vm.Interrupt(errMemory)
				return
			}
		}
	}
}

func heapBytes() uint64 {
	sample := []metrics.Sample{{Name: heapObjectsKey}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

func (r *scriptRun) call(name string, arg goja.Value, call func(string, json.RawMessage) (json.RawMessage, error)) goja.Value {
	promise, resolve, reject := r.vm.NewPromise()
	result, err := r.invoke(name, arg, call)
	if err != nil {
		_ = reject(r.newError(err.Error()))
	} else {
		_ = resolve(result)
	}
	return r.vm.ToValue(promise)
}

func (r *scriptRun) invoke(name string, arg goja.Value, call func(string, json.RawMessage) (json.RawMessage, error)) (goja.Value, error) {
	args := json.RawMessage("{}")
	if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		encoded, err := r.stringify(goja.Undefined(), arg)
		if err != nil {
			return nil, fmt.Errorf("arguments are not JSON: %w", err)
		}
		args = json.RawMessage(encoded.String())
	}
	result, err := call(name, args)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return goja.Undefined(), nil
	}
	return r.parse(goja.Undefined(), r.vm.ToValue(string(result)))
}

func (r *scriptRun) newError(msg string) goja.Value {
	obj, err := r.vm.New(r.vm.Get("Error"), r.vm.ToValue(msg))
	if err != nil {
		return r.vm.ToValue(msg)
	}
	return obj
}

func (r *scriptRun) format(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return ""
	}
	if s, ok := v.Export().(string); ok {
		return s
	}
	encoded, err := r.stringify(goja.Undefined(), v, goja.Null(), r.vm.ToValue(2))
	if err != nil || goja.IsUndefined(encoded) {
		return v.String()
	}
	return encoded.String()
}

func (r *scriptRun) describeError(v goja.Value) string {
	msg := v.String()
	obj, ok := v.(*goja.Object)
	if !ok {
		return msg
	}
	stack := obj.Get("stack")
	if stack == nil || goja.IsUndefined(stack) {
		return msg
	}
	s := stack.String()
	if strings.HasPrefix(s, msg) {
		return s
	}
	return msg + "\n" + strings.TrimSpace(s)
}

func (r *scriptRun) output(result string) string {
	var b strings.Builder
	b.WriteString(result)
	if len(r.logs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("console:\n")
		b.WriteString(strings.Join(r.logs, "\n"))
	}
	out := b.String()
	if len(out) > maxOutput {
		out = strings.ToValidUTF8(out[:maxOutput], "") + "\n[output truncated: return less data]"
	}
	return out
}
