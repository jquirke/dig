// Copyright (c) 2021 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dig

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.uber.org/dig/internal/digreflect"
	"go.uber.org/dig/internal/dot"
	"go.uber.org/dig/internal/graph"
)

// A ProvideOption modifies the default behavior of Provide.
type ProvideOption interface {
	applyProvideOption(*provideOptions)
}

type provideOptions struct {
	Name     string
	Group    string
	Info     *ProvideInfo
	As       []interface{}
	Location *digreflect.Func
}

func (o *provideOptions) Validate() error {
	if len(o.Group) > 0 {
		if len(o.Name) > 0 {
			return fmt.Errorf(
				"cannot use named values with value groups: name:%q provided with group:%q", o.Name, o.Group)
		}
		if len(o.As) > 0 {
			return fmt.Errorf(
				"cannot use dig.As with value groups: dig.As provided with group:%q", o.Group)
		}
	}

	// Names must be representable inside a backquoted string. The only
	// limitation for raw string literals as per
	// https://golang.org/ref/spec#raw_string_lit is that they cannot contain
	// backquotes.
	if strings.ContainsRune(o.Name, '`') {
		return errf("invalid dig.Name(%q): names cannot contain backquotes", o.Name)
	}
	if strings.ContainsRune(o.Group, '`') {
		return errf("invalid dig.Group(%q): group names cannot contain backquotes", o.Group)
	}

	for _, i := range o.As {
		t := reflect.TypeOf(i)

		if t == nil {
			return fmt.Errorf("invalid dig.As(nil): argument must be a pointer to an interface")
		}

		if t.Kind() != reflect.Ptr {
			return fmt.Errorf("invalid dig.As(%v): argument must be a pointer to an interface", t)
		}

		pointingTo := t.Elem()
		if pointingTo.Kind() != reflect.Interface {
			return fmt.Errorf("invalid dig.As(*%v): argument must be a pointer to an interface", pointingTo)
		}
	}
	return nil
}

type provideOptionFunc func(*provideOptions)

func (f provideOptionFunc) applyProvideOption(opts *provideOptions) { f(opts) }

// Name is a ProvideOption that specifies that all values produced by a
// constructor should have the given name. See also the package documentation
// about Named Values.
//
// Given,
//
//   func NewReadOnlyConnection(...) (*Connection, error)
//   func NewReadWriteConnection(...) (*Connection, error)
//
// The following will provide two connections to the container: one under the
// name "ro" and the other under the name "rw".
//
//   c.Provide(NewReadOnlyConnection, dig.Name("ro"))
//   c.Provide(NewReadWriteConnection, dig.Name("rw"))
//
// This option cannot be provided for constructors which produce result
// objects.
func Name(name string) ProvideOption {
	return provideOptionFunc(func(opts *provideOptions) {
		opts.Name = name
	})
}

// Group is a ProvideOption that specifies that all values produced by a
// constructor should be added to the specified group. See also the package
// documentation about Value Groups.
//
// This option cannot be provided for constructors which produce result
// objects.
func Group(group string) ProvideOption {
	return provideOptionFunc(func(opts *provideOptions) {
		opts.Group = group
	})
}

// ID is a unique integer representing the constructor node in the dependency graph.
type ID int

// ProvideInfo provides information about the constructor's inputs and outputs
// types as strings, as well as the ID of the constructor supplied to the Container.
// It contains ID for the constructor, as well as slices of Input and Output types,
// which are Stringers that report the types of the parameters and results respectively.
type ProvideInfo struct {
	ID      ID
	Inputs  []*Input
	Outputs []*Output
}

// Input contains information on an input parameter of the constructor.
type Input struct {
	t           reflect.Type
	optional    bool
	name, group string
}

func (i *Input) String() string {
	toks := make([]string, 0, 3)
	t := i.t.String()
	if i.optional {
		toks = append(toks, "optional")
	}
	if i.name != "" {
		toks = append(toks, fmt.Sprintf("name = %q", i.name))
	}
	if i.group != "" {
		toks = append(toks, fmt.Sprintf("group = %q", i.group))
	}

	if len(toks) == 0 {
		return t
	}
	return fmt.Sprintf("%v[%v]", t, strings.Join(toks, ", "))
}

// Output contains information on an output produced by the constructor.
type Output struct {
	t           reflect.Type
	name, group string
}

func (o *Output) String() string {
	toks := make([]string, 0, 2)
	t := o.t.String()
	if o.name != "" {
		toks = append(toks, fmt.Sprintf("name = %q", o.name))
	}
	if o.group != "" {
		toks = append(toks, fmt.Sprintf("group = %q", o.group))
	}

	if len(toks) == 0 {
		return t
	}
	return fmt.Sprintf("%v[%v]", t, strings.Join(toks, ", "))
}

// FillProvideInfo is a ProvideOption that writes info on what Dig was able to get out
// out of the provided constructor into the provided ProvideInfo.
func FillProvideInfo(info *ProvideInfo) ProvideOption {
	return provideOptionFunc(func(opts *provideOptions) {
		opts.Info = info
	})
}

// As is a ProvideOption that specifies that the value produced by the
// constructor implements one or more other interfaces and is provided
// to the container as those interfaces.
//
// As expects one or more pointers to the implemented interfaces. Values
// produced by constructors will be then available in the container as
// implementations of all of those interfaces, but not as the value itself.
//
// For example, the following will make io.Reader and io.Writer available
// in the container, but not buffer.
//
//   c.Provide(newBuffer, dig.As(new(io.Reader), new(io.Writer)))
//
// That is, the above is equivalent to the following.
//
//   c.Provide(func(...) (io.Reader, io.Writer) {
//     b := newBuffer(...)
//     return b, b
//   })
//
// If used with dig.Name, the type produced by the constructor and the types
// specified with dig.As will all use the same name. For example,
//
//   c.Provide(newFile, dig.As(new(io.Reader)), dig.Name("temp"))
//
// The above is equivalent to the following.
//
//   type Result struct {
//     dig.Out
//
//     Reader io.Reader `name:"temp"`
//   }
//
//   c.Provide(func(...) Result {
//     f := newFile(...)
//     return Result{
//       Reader: f,
//     }
//   })
//
// This option cannot be provided for constructors which produce result
// objects.
func As(i ...interface{}) ProvideOption {
	return provideOptionFunc(func(opts *provideOptions) {
		opts.As = append(opts.As, i...)
	})
}

// LocationForPC is a ProvideOption which specifies an alternate function program
// counter address to be used for debug information. The package, name, file and
// line number of this alternate function address will be used in error messages
// and DOT graphs. This option is intended to be used with functions created
// with the reflect.MakeFunc method whose error messages are otherwise hard to
// understand
func LocationForPC(pc uintptr) ProvideOption {
	return provideOptionFunc(func(opts *provideOptions) {
		opts.Location = digreflect.InspectFuncPC(pc)
	})
}

// provider encapsulates a user-provided constructor.
type provider interface {
	// ID is a unique numerical identifier for this provider.
	ID() dot.CtorID

	// Order reports the order of this provider in the graphHolder.
	// This value is usually returned by the graphHolder.NewNode method.
	Order() int

	// Location returns where this constructor was defined.
	Location() *digreflect.Func

	// ParamList returns information about the direct dependencies of this
	// constructor.
	ParamList() paramList

	// ResultList returns information about the values produced by this
	// constructor.
	ResultList() resultList

	// Calls the underlying constructor, reading values from the
	// containerStore as needed.
	//
	// The values produced by this provider should be submitted into the
	// containerStore.
	Call(containerStore) error

	CType() reflect.Type
}

// Provide teaches the container how to build values of one or more types and
// expresses their dependencies.
//
// The first argument of Provide is a function that accepts zero or more
// parameters and returns one or more results. The function may optionally
// return an error to indicate that it failed to build the value. This
// function will be treated as the constructor for all the types it returns.
// This function will be called AT MOST ONCE when a type produced by it, or a
// type that consumes this function's output, is requested via Invoke. If the
// same types are requested multiple times, the previously produced value will
// be reused.
//
// In addition to accepting constructors that accept dependencies as separate
// arguments and produce results as separate return values, Provide also
// accepts constructors that specify dependencies as dig.In structs and/or
// specify results as dig.Out structs.
func (c *Container) Provide(constructor interface{}, opts ...ProvideOption) error {
	ctype := reflect.TypeOf(constructor)
	if ctype == nil {
		return errors.New("can't provide an untyped nil")
	}
	if ctype.Kind() != reflect.Func {
		return errf("must provide constructor function, got %v (type %v)", constructor, ctype)
	}

	var options provideOptions
	for _, o := range opts {
		o.applyProvideOption(&options)
	}
	if err := options.Validate(); err != nil {
		return err
	}

	if err := c.provide(constructor, options); err != nil {
		return errProvide{
			Func:   digreflect.InspectFunc(constructor),
			Reason: err,
		}
	}
	return nil
}

func (c *Container) provide(ctor interface{}, opts provideOptions) (err error) {
	// take a snapshot of the current graph state before
	// we start making changes to it as we may need to
	// undo them upon encountering errors.
	c.gh.Snapshot()
	defer func() {
		if err != nil {
			c.gh.Rollback()
		}
	}()

	n, err := newConstructorNode(
		ctor,
		c,
		constructorOptions{
			ResultName:  opts.Name,
			ResultGroup: opts.Group,
			ResultAs:    opts.As,
			Location:    opts.Location,
		},
	)
	if err != nil {
		return err
	}

	keys, err := c.findAndValidateResults(n)
	if err != nil {
		return err
	}

	ctype := reflect.TypeOf(ctor)
	if len(keys) == 0 {
		return errf("%v must provide at least one non-error type", ctype)
	}

	oldProviders := make(map[key][]*constructorNode)
	for k := range keys {
		// Cache old providers before running cycle detection.
		oldProviders[k] = c.providers[k]
		c.providers[k] = append(c.providers[k], n)
	}

	c.isVerifiedAcyclic = false
	if !c.deferAcyclicVerification {
		if ok, cycle := graph.IsAcyclic(c.gh); !ok {
			// When a cycle is detected, recover the old providers to reset
			// the providers map back to what it was before this node was
			// introduced.
			for k, ops := range oldProviders {
				c.providers[k] = ops
			}

			return errf("this function introduces a cycle", c.cycleDetectedError(cycle))
		}
		c.isVerifiedAcyclic = true
	}
	c.nodes = append(c.nodes, n)

	// Record introspection info for caller if Info option is specified
	if info := opts.Info; info != nil {
		params := n.ParamList().DotParam()
		results := n.ResultList().DotResult()

		info.ID = (ID)(n.id)
		info.Inputs = make([]*Input, len(params))
		info.Outputs = make([]*Output, len(results))

		for i, param := range params {
			info.Inputs[i] = &Input{
				t:        param.Type,
				optional: param.Optional,
				name:     param.Name,
				group:    param.Group,
			}
		}

		for i, res := range results {
			info.Outputs[i] = &Output{
				t:     res.Type,
				name:  res.Name,
				group: res.Group,
			}
		}
	}
	return nil
}

// Builds a collection of all result types produced by this constructor.
func (c *Container) findAndValidateResults(n *constructorNode) (map[key]struct{}, error) {
	var err error
	keyPaths := make(map[key]string)
	walkResult(n.ResultList(), connectionVisitor{
		c:        c,
		n:        n,
		err:      &err,
		keyPaths: keyPaths,
	})

	if err != nil {
		return nil, err
	}

	keys := make(map[key]struct{}, len(keyPaths))
	for k := range keyPaths {
		keys[k] = struct{}{}
	}
	return keys, nil
}

// Visits the results of a node and compiles a collection of all the keys
// produced by that node.
type connectionVisitor struct {
	c *Container
	n *constructorNode

	// If this points to a non-nil value, we've already encountered an error
	// and should stop traversing.
	err *error

	// Map of keys provided to path that provided this. The path is a string
	// documenting which positional return value or dig.Out attribute is
	// providing this particular key.
	//
	// For example, "[0].Foo" indicates that the value was provided by the Foo
	// attribute of the dig.Out returned as the first result of the
	// constructor.
	keyPaths map[key]string

	// We track the path to the current result here. For example, this will
	// be, ["[1]", "Foo", "Bar"] when we're visiting Bar in,
	//
	//   func() (io.Writer, struct {
	//     dig.Out
	//
	//     Foo struct {
	//       dig.Out
	//
	//       Bar io.Reader
	//     }
	//   })
	currentResultPath []string
}

func (cv connectionVisitor) AnnotateWithField(f resultObjectField) resultVisitor {
	cv.currentResultPath = append(cv.currentResultPath, f.FieldName)
	return cv
}

func (cv connectionVisitor) AnnotateWithPosition(i int) resultVisitor {
	cv.currentResultPath = append(cv.currentResultPath, fmt.Sprintf("[%d]", i))
	return cv
}

func (cv connectionVisitor) Visit(res result) resultVisitor {
	// Already failed. Stop looking.
	if *cv.err != nil {
		return nil
	}

	path := strings.Join(cv.currentResultPath, ".")

	switch r := res.(type) {

	case resultSingle:
		k := key{name: r.Name, t: r.Type}

		if err := cv.checkKey(k, path); err != nil {
			*cv.err = err
			return nil
		}
		for _, asType := range r.As {
			k := key{name: r.Name, t: asType}
			if err := cv.checkKey(k, path); err != nil {
				*cv.err = err
				return nil
			}
		}

	case resultGrouped:
		// we don't really care about the path for this since conflicts are
		// okay for group results. We'll track it for the sake of having a
		// value there.
		k := key{group: r.Group, t: r.Type}
		cv.keyPaths[k] = path
	}

	return cv
}

func (cv connectionVisitor) checkKey(k key, path string) error {
	defer func() { cv.keyPaths[k] = path }()
	if conflict, ok := cv.keyPaths[k]; ok {
		return errf(
			"cannot provide %v from %v", k, path,
			"already provided by %v", conflict,
		)
	}
	if ps := cv.c.providers[k]; len(ps) > 0 {
		cons := make([]string, len(ps))
		for i, p := range ps {
			cons[i] = fmt.Sprint(p.Location())
		}

		return errf(
			"cannot provide %v from %v", k, path,
			"already provided by %v", strings.Join(cons, "; "),
		)
	}
	return nil
}