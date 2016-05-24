// Package ops provides a facility for tracking the processing of operations,
// including contextual metadata about the operation and their final success or
// failure. An op is assumed to have succeeded if by the time of calling Exit()
// no errors have been reported. The final status can be reported to a metrics
// facility.
package ops

import (
	"sync"

	"github.com/getlantern/context"
)

var (
	reporters      []Reporter
	reportersMutex sync.RWMutex
)

// Reporter is a function that reports the success or failure of an Op. If
// failure is nil, the Op can be considered successful.
type Reporter func(failure error, ctx map[string]interface{})

// Op represents an operation that's being performed. It mimics the API of
// context.Context.
type Op interface {
	// Enter enters a new level on this Op's Context stack.
	Enter(name string) Op

	// Go starts the given function on a new goroutine.
	Go(fn func())

	// Exit exits the current level on this Context stack.
	Exit() Op

	// Put puts a key->value pair into the current level of the context stack.
	Put(key string, value interface{}) Op

	// PutDynamic puts a key->value pair into the current level of the context stack
	// where the value is generated by a function that gets evaluated at every Read.
	PutDynamic(key string, valueFN func() interface{}) Op

	// FailOnError marks this op as failed if the given err is not nil. If
	// FailOnError is called multiple times, the latest error will be reported as
	// the failure. Returns the original error for convenient chaining.
	FailOnError(err error) error
}

type op struct {
	ctx     context.Context
	failure error
}

// RegisterReporter registers the given reporter.
func RegisterReporter(reporter Reporter) {
	reportersMutex.Lock()
	reporters = append(reporters, reporter)
	reportersMutex.Unlock()
}

// Enter enters a new level on the current Op's Context stack, creating a new Op
// if necessary.
func Enter(name string) Op {
	return &op{ctx: context.Enter().Put("op", name).PutIfAbsent("root_op", name)}
}

func (o *op) Enter(name string) Op {
	return &op{ctx: o.ctx.Enter().Put("op", name).PutIfAbsent("root_op", name)}
}

func (o *op) Go(fn func()) {
	o.ctx.Go(fn)
}

// Go mimics the function from context.
func Go(fn func()) {
	context.Go(fn)
}

func (o *op) Exit() Op {
	var reportersCopy []Reporter
	reportersMutex.RLock()
	if len(reporters) > 0 {
		reportersCopy = make([]Reporter, len(reporters))
		copy(reportersCopy, reporters)
	}
	reportersMutex.RUnlock()

	if len(reportersCopy) > 0 {
		ctx := o.ctx.AsMap(o.failure, true)
		for _, reporter := range reportersCopy {
			reporter(o.failure, ctx)
		}
	}
	return &op{ctx: o.ctx.Exit()}
}

func (o *op) Put(key string, value interface{}) Op {
	o.ctx.Put(key, value)
	return o
}

func (o *op) PutDynamic(key string, valueFN func() interface{}) Op {
	o.ctx.PutDynamic(key, valueFN)
	return o
}

func (o *op) FailOnError(err error) error {
	if err != nil {
		o.failure = err
	}
	return err
}
