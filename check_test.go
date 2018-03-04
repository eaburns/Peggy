// Copyright 2017 The Peggy Authors
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name string
		in   string
		err  string
	}{
		{
			name: "empty OK",
			in:   "",
			err:  "",
		},
		{
			name: "various OK",
			in: `A <- (G/B C)*
B <- &{pred}*
C <- !{pred}* T:{ act }
D <- .* !B
E <- C*
F <- "cde"*
G <- [fgh]*`,
			err: "",
		},
		{
			name: "redefined rule",
			in:   "A <- [x]\nA <- [y]",
			err:  "^test.file:2.1,2.9: rule A redefined",
		},
		{
			name: "undefined rule",
			in:   "A <- B",
			err:  "^test.file:1.6,1.7: rule B undefined",
		},
		{
			name: "redefined label",
			in:   "A <- a:[a] a:[a]",
			err:  "^test.file:1.12,1.13: label a redefined",
		},
		{
			name: "choice first error",
			in:   "A <- Undefined / A",
			err:  ".+",
		},
		{
			name: "choice second error",
			in:   "A <- B / Undefined\nB <- [x]",
			err:  ".+",
		},
		{
			name: "seq first error",
			in:   "A <- Undefined A",
			err:  ".+",
		},
		{
			name: "sequence second error",
			in:   "A <- B Undefined\nB <- [x]",
			err:  ".+",
		},
		{
			name: "template parameter OK",
			in: `A<x> <- x
				B <- A<C>
				C <- "c"`,
			err: "",
		},
		{
			name: "template parameter redef",
			in: `A<x, x> <- x
				B <- A<C, C>
				C <- "c"`,
			err: "^test.file:1.6,1.7: parameter x redefined$",
		},
		{
			name: "template and non-template redef",
			in: `A<x> <- x
				B <- A<C>
				C <- "c"
				A <- "a"`,
			err: "^test.file:4.5,4.13: rule A redefined$",
		},
		{
			name: "template arg count mismatch",
			in: `A<x> <- x
				B <- A<C, C>
				C <- "c"`,
			err: "test.file:2.10,2.16: template A<x> argument count mismatch: got 2, expected 1",
		},
		{
			name: "multiple errors",
			in:   "A <- U1 U2\nA <- u:[x] u:[x]",
			err: "test.file:1.6,1.8: rule U1 undefined\n" +
				"test.file:1.9,1.11: rule U2 undefined\n" +
				"test.file:2.1,2.17: rule A redefined\n" +
				"test.file:2.12,2.13: label u redefined",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			in := strings.NewReader(test.in)
			g, err := Parse(in, "test.file")
			if err != nil {
				t.Errorf("Parse(%q, _)=_, %v, want _,nil", test.in, err)
				return
			}
			err = Check(g)
			if test.err == "" {
				if err != nil {
					t.Errorf("Check(%q)=%v, want nil", test.in, err)
				}
				return
			}
			re := regexp.MustCompile(test.err)
			if err == nil || !re.MatchString(err.Error()) {
				var e string
				if err != nil {
					e = err.Error()
				}
				t.Errorf("Check(%q)=%v, but expected to match %q",
					test.in, e, test.err)
				return
			}
		})
	}
}
