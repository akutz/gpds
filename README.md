# Go Plug-ins & Vendored Dependencies: A Solution
This document outlines a solution for the problem described in
[golang/go#20481](https://github.com/golang/go/issues/20481). Please
review the original problem description before continuing. This project
is [proposed](https://github.com/golang/go/issues/20481#issuecomment-305355306)
as a solution in the original issue.

The problem was fairly straight-forward and ultimately so is the solution:
a Go plug-in should not depend on a host process's symbols. That means:
* Go plug-ins should use a unidirectional model for type registration
* Go plug-ins should use `interface{}` for all non-stdlib types involved
in ingress and egress host-plug-in communications

## Unidirectional Model
Go supports the dependency inversion principle
([DIP](https://en.wikipedia.org/wiki/Dependency_inversion_principle))
through the use of interface abstractions, but there still must
exist a mechanism to provide implementations of the abstractions on
which a program depends. One such solution can be found in the list
of suggested implementations of inversion of control
([IoC](https://en.wikipedia.org/wiki/Inversion_of_control)): the
[service locator pattern](https://en.wikipedia.org/wiki/Service_locator_pattern).

The service locator pattern is very easy to implement in Go as a simple
type registry. Consumers that require an implementation of some interface
are able to query the type registry and receive an object instance that
fulfills the abstraction. There are two models that can be used to
prime the registry with types: bidirectional and unidirectional.

![Bidirectional Relationship](http://svgshare.com/i/1nf.svg "Bidirectional Relationship")

The above diagram is an example of the bidirectional model, but it fails
when used in concert with Go plug-ins due to the issues with dependencies
outlined in [golang/go#20481](https://github.com/golang/go/issues/20481). The
solution is a unidirectional model:

![Unidirectional Relationship](http://svgshare.com/i/1o0.svg "Unidirectional Relationship")

Illustrated in the diagram above, the unidirectional model provides the same
type registry that the bidirectional model does but relocates type registration
from the plug-ins' `init` functions to the host process. This change means the
plug-ins no longer depend on the type registry in the host process, and that's
very important because a plug-in cannot depend on a host process's symbols.

## Interface In / Interface Out
Go interfaces are really powerful, but they are also quick to cause issues when
used with plug-ins for two reasons:

1. Interface equality is not as simple as it seems
2. The fully-qualified path to an interface matters

### Interface Equality
The following examples demonstrate the power and peril of using Go
interfaces interchangeably having assumed equality. The first example
defines two, identical interfaces, `dog` and `fox`, and two structs,
best friends that implement the interfaces, `copper` and `todd`
([run example](https://play.golang.org/p/4fZRGr2qgj)):

```go
package main

import (
	"fmt"
)

type dog interface {
	bark() string
}

type fox interface {
	bark() string
}

type copper struct{}

func (c *copper) bark() string { return "woof!" }

type todd struct{}

func (t *todd) bark() string { return "woof!" }

func barkWithDog(d dog) { fmt.Println(d.bark()) }
func barkWithFox(f fox) { fmt.Println(f.bark()) }

func main() {
	var d dog = &copper{}
	var f fox = &todd{}
	barkWithDog(d)
	barkWithFox(f)
}
```

The above code, when executed, will print `woof!` on two lines. The
first line is the result of the dog Copper barking, and the second
line is his friend Todd the fox taking a turn. However, what makes
Copper a dog or Todd a fox? According to the code it's because
`copper` implements the function `bark() string` from the `dog`
interface and `todd` implements the same function from the `fox`
interface.

Does that mean that `copper` and `todd` are interchangeable? In fact,
the two friends decided to pretend to be one another in order to play a
trick on the kind old lady and hunter
([run example](https://play.golang.org/p/LDNCODWctg)):

```go
func main() {
	var d dog = &todd{}
	var f fox = &copper{}
	barkWithDog(f)
	barkWithFox(d)
}
```

How can Todd be a fox and Copper a dog? According to Go's interface
rules, a variable of type `fox` can be assigned any type that
implements the `bark() string` function. A function that has an
argument of type `dog` or `fox` can also accept any type that implements
the `bark() string` function, even if that type is another interface.

It would appear then that multiple Go interfaces, if they define the same
abstraction, are identical. However, thanks to Go's strong type
system, interfaces are not as interchangeable as they first appear
([run example](https://play.golang.org/p/RT3X1Ot2WS)):

```go
package main

import (
	"fmt"
)

type dog interface {
	bark() string
	same(d dog) bool
}

type fox interface {
	bark() string
	same(f fox) bool
}

type copper struct{}

func (c *copper) bark() string    { return "woof!" }
func (c *copper) same(d dog) bool { return c == d }

type todd struct{}

func (t *todd) bark() string    { return "woof!" }
func (t *todd) same(f fox) bool { return t == f }

func barkWithDog(d dog) { fmt.Println(d.bark()) }
func barkWithFox(f fox) { fmt.Println(f.bark()) }

func main() {
	var d dog = &todd{}
	var f fox = &copper{}
	barkWithDog(f)
	barkWithFox(d)
}
```

The above example will no longer emit the sound of two friends barking,
but rather the following errors:

```bash
tmp/sandbox006620983/main.go:31: cannot use todd literal (type *todd) as type dog in assignment:
	*todd does not implement dog (wrong type for same method)
		have same(fox) bool
		want same(dog) bool
tmp/sandbox006620983/main.go:32: cannot use copper literal (type *copper) as type fox in assignment:
	*copper does not implement fox (wrong type for same method)
		have same(dog) bool
		want same(fox) bool
tmp/sandbox006620983/main.go:33: cannot use f (type fox) as type dog in argument to barkWithDog:
	fox does not implement dog (wrong type for same method)
		have same(fox) bool
		want same(dog) bool
tmp/sandbox006620983/main.go:34: cannot use d (type dog) as type fox in argument to barkWithFox:
	dog does not implement fox (wrong type for same method)
		have same(dog) bool
		want same(fox) bool
```

The relevant piece of information from the above error text is the following:

```bash
have same(fox) bool
want same(dog) bool
```

In other words, even though Go interfaces `A` and `B` are identical, `A{A}` and
`B{B}` are not. If `A`==`B` and `C`==`D`, `A{C}` != `B{D}`.

Because of this rule, without a shared types library, even with Go interfaces,
it's not possible for Go plug-ins to expect to share or use symbols provided
by the host process.

### Fully-Qualified Type Path
However, even redefining interfaces inside plug-ins to match types found in
the host process will fail if those interfaces are used by exported symbols.
This section uses this project's `dog` package. The following command will
get the package and build its plug-ins:

```bash
$ go get github.com/akutz/gpds && \
  cd $GOPATH/src/github.com/akutz/gpds/dog && \
  for d in $(find . -maxdepth 1 -type d | grep -v '^.$'); do \
    go build -buildmode plugin -o $d.so $d; \
  done
```

Run the program using the `sit.so` plug-in:

```bash
$ go run main.go dog.go sit.so
error: invalid Command func: func(main.dog)
exit status 1
```

An error occurs because the `sit.so` plug-in defines an interface `dog`
to match the host program's interface `Dog`. Both interfaces include a
single function: `Name() string`. However, these types are different
because their fully-qualified type paths (FQTP) are not the same. An
FQTP includes the package path to a type and the type's name, where
the name is case sensitive (since case sensitivity is used by Go to
indicate public and private members).

Therefore invoking the `Command(Dog)` function fails, because while
the interface definitions are identical with regards to the equality
ruleset outlined above, the two interfaces do not have the same FQTP.

#### Curious Exception
There is one curious exception to this rule: when an interface is defined
in the `main` package of the host program as well as the `main` package of
the plug-in. Run the program using the `speak.so` plug-in:

```bash
$ go run main.go dog.go speak.so
Lucy
```

The program should have printed the name "Lucy". However, if the code
is examined, the `Dog` interface is defined in both the host program
**and** in the plug-in package. Yet it works. Why? The answer is
almost so embarrassingly obvious that it makes this author hesitant to
admit it took him an hour of looking at the problem to figure it out.

Both interfaces have a fully-qualified package path of `main.Dog`.

When interfaces are defined in the `main` package of the hosting
program and in the `main` package of a plug-in, their symbols are
identical. However, like most things, there's an exception to even this.

#### Exception to the Exception
What happens if the `Dog` interface references itself? The answer is an
error this author has never seen before in his history of working with
the Go programming language. To reproduce this error, run the program
using the `stay.so` plug-in:

```bash
$ go run main.go self.go stay.so
runtime: goroutine stack exceeds 1000000000-byte limit
fatal error: stack overflow

runtime stack:
runtime.throw(0x534aad, 0xe)
	/home/akutz/.go/1.8.1/src/runtime/panic.go:596 +0x95
runtime.newstack(0x0)
	/home/akutz/.go/1.8.1/src/runtime/stack.go:1089 +0x3f2
runtime.morestack()
	/home/akutz/.go/1.8.1/src/runtime/asm_amd64.s:398 +0x86

goroutine 1 [running]:
runtime.(*_type).string(0x7f2488279520, 0x0, 0x0)
	/home/akutz/.go/1.8.1/src/runtime/type.go:45 +0xad fp=0xc44009c358 sp=0xc44009c350
runtime.typesEqual(0x7f2488279520, 0x51d0c0, 0x50a310)
	/home/akutz/.go/1.8.1/src/runtime/type.go:543 +0x73 fp=0xc44009c480 sp=0xc44009c358
runtime.typesEqual(0x7f2488270740, 0x5137c0, 0x5137c0)
	/home/akutz/.go/1.8.1/src/runtime/type.go:586 +0x368 fp=0xc44009c5a8 sp=0xc44009c480
runtime.typesEqual(0x7f2488279520, 0x51d0c0, 0x50a310)
	/home/akutz/.go/1.8.1/src/runtime/type.go:615 +0x740 fp=0xc44009c6d0 sp=0xc44009c5a8
...additional frames elided...

goroutine 17 [syscall, locked to thread]:
runtime.goexit()
	/home/akutz/.go/1.8.1/src/runtime/asm_amd64.s:2197 +0x1
exit status 2
```

The above program fails due to a Go runtime panic where Go is recursively trying
to determine if the `main.Dog` interface from the host program is the same
type as the `main.Dog` interface defined in the plug-in. The interfaces were
considered the same when they did not reference themselves with their respective
`Self() Dog` functions.

## The Solution
The proposed solution adheres to the crucial restriction outlined at the
beginning of this document -- Go plug-ins should not depend on a
host program's symbols. This project is used to demonstrate a program and
plug-ins that:
* Use a unidirectional model for type registration
* Use `interface{}` for all non-stdlib types involved in ingress and egress
host-plug-in communications

To get started please clone this repository:

```bash
$ go get github.com/akutz/gpds && cd $GOPATH/src/github.com/akutz/gpds
```

Running the program will emit a little ditty by Mr. Chapin:

```bash
$ go run main.go
Yes, we have no bananas,
We have no bananas today.
```

Next, build the plug-in `mod.so`:

```bash
$ go build -buildmode plugin -o mod.so ./mod
```

Running the program with the plug-in will cause the output to
be somewhat altered:

```bash
$ go run main.go mod.so
*main.v2Config
Yes there were thirty, thousand, pounds...
Of...bananas.

*main.v2Config
Bottom-line, sh*t kicking country choir
You'll see your part come by
```

The above steps do not appear to illustrate anything too fancy, but
under the covers is a model that enables Go plug-ins to work alongside
vendoered dependencies with ease. Pulling back the covers ever so slightly
reveals how it all works.

### Lib
For starters, this is the `Module` interface defined in `./lib/lib.go`:

```go
// Module is the interface implemented by types that
// register themselves as modular plug-ins.
type Module interface {

	// Init initializes the module.
	//
	// The config argument can be asserted as an implementation of the
	// of the github.com/akutz/gpds/lib/v2.Config interface or older.
	Init(ctx context.Context, config interface{}) error
}
```

A `Module` interface includes a single function, `Init`, which accepts
a Go context and second argument of type `interface{}`. The second
argument is expected to be a sort of configuration provider, provided to
plug-ins to inform their initialization routine. Please note the Godoc
for the argument:

> The config argument can be asserted as an implementation of the
> of the github.com/akutz/gpd/lib/v2.Config interface or older.

The documentation indicates which type the argument can be asserted as,
and more importantly explains that the object provided can be asserted
as a specific version of that type **or older**. All types should be
versioned and plug-ins that assert v1.Type should continue to work
even if v1+.Type is provided.

Please note that this model could be further enhanced so that plug-ins
provide a symbol that contains the expected API version so that host
programs can eventually deprecate older types by restricting which
plug-ins get loaded based on their supported API type.

### Mod
The file `./mod/mod.go` is the core of the plug-in. At the top of the
file is the `Types` symbol:

```go
// Types is the symbol the host process uses to
// retrieve the plug-in's type map
var Types = map[string]func() interface{}{
	"mod_go": func() interface{} { return &module{} },
}
```

The `Types` symbol is very important, and the model proposed in this project
expects all plug-ins to define this symbol. The symbol is a type map that
is used by the host program to register the plug-in's type names and
functions to construct them.

The module `mod_go` referenced in the plug-in's type map looks like this:

```go
type module struct{}

func (m *module) Init(ctx context.Context, configObj interface{}) error {
	config, configOk := configObj.(Config)
	if !configOk {
		return errInvalidConfig
	}
	fmt.Fprintf(os.Stdout, "%T\n", config)
	fmt.Fprintln(os.Stdout, config.Get(ctx, "bananas"))
	return nil
}
```

Since this is an example the plug-in's module only defines a barebones
implementation of the `Module` interface. The module's `Init` function
first asserts that the provided `configObj` argument can be asserted
as the `Config` interface and then uses the typed object to retrieve
and print `bananas`.

But wait, how is the plug-in able to assert the interface `Config` if
the plug-in is not sharing any symbols with the host process? Simple,
the plug-in simply treats the sources in the versioned `lib` package
as C headers and copies the `v1` or `v2` *headers* into the plug-in's
own package.

## Conclusion
Hopefully this document has not only shown how to solve the issue of Go
plug-ins and vendored dependencies, but also clearly articulated the
reasoning behind the decisions that led to the proposed solution.
