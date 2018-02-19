package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/eaburns/pretty"
)

type actionTest struct {
	name    string
	grammar string
	cases   []actionTestCase
}

type actionTestCase struct {
	input string
	want  interface{}
}

var actionTests = []actionTest{
	{
		name:    "literal",
		grammar: `A <- "abc☺XYZ"`,
		cases: []actionTestCase{
			{"abc☺XYZ", "abc☺XYZ"},
		},
	},
	{
		name:    "char class",
		grammar: `A <- [a-zA-Z0-9☺]`,
		cases: []actionTestCase{
			{"a", "a"},
			{"☺", "☺"},
			{"Z", "Z"},
			{"5", "5"},
		},
	},
	{
		name:    "any char",
		grammar: `A <- .`,
		cases: []actionTestCase{
			{"a", "a"},
			{"☺", "☺"},
			{"Z", "Z"},
			{"5", "5"},
		},
	},
	{
		name:    "star",
		grammar: `A <- "abc"*`,
		cases: []actionTestCase{
			{"", nil},
			{"abc", []interface{}{"abc"}},
			{"abcabc", []interface{}{"abc", "abc"}},
			{"abcabcabcabc", []interface{}{"abc", "abc", "abc", "abc"}},
		},
	},
	{
		name:    "plus",
		grammar: `A <- "abc"+`,
		cases: []actionTestCase{
			{"abc", []interface{}{"abc"}},
			{"abcabc", []interface{}{"abc", "abc"}},
			{"abcabcabcabc", []interface{}{"abc", "abc", "abc", "abc"}},
		},
	},
	{
		name:    "question",
		grammar: `A <- "abc"?`,
		cases: []actionTestCase{
			{"", nil},
			{"abc", "abc"},
		},
	},
	{
		name:    "single type sequence",
		grammar: `A <- "a" "b" "c"`,
		cases: []actionTestCase{
			{"abc", []interface{}{"a", "b", "c"}},
		},
	},
	{
		name:    "single type choice",
		grammar: `A <- "abc" / "☺☹" / .`,
		cases: []actionTestCase{
			{"abc", "abc"},
			{"☺☹", "☺☹"},
			{"z", "z"},
		},
	},
	{
		name:    "multi-type choice",
		grammar: `A <- "abc" / "x" "y" "z"`,
		cases: []actionTestCase{
			{"abc", "abc"},
			{"xyz", []interface{}{"x", "y", "z"}},
		},
	},
	{
		name:    "multi-type sequence",
		grammar: `A <- ("a" "b" "c") "xyz"`,
		cases: []actionTestCase{
			{"abcxyz", []interface{}{
				[]interface{}{"a", "b", "c"},
				"xyz"},
			},
		},
	},
	{
		name: "identifier",
		grammar: `
			A <- Abc "xyz"
			Abc <- "a" "b" "c"`,
		cases: []actionTestCase{
			{"abcxyz", []interface{}{
				[]interface{}{"a", "b", "c"},
				"xyz"},
			},
		},
	},
	{
		name: "true predicate",
		grammar: `
			A <- &Abc "abc"
			Abc <- "a" "b" "c"`,
		cases: []actionTestCase{
			{"abc", []interface{}{true, "abc"}},
		},
	},
	{
		name: "false predicate",
		grammar: `
			A <- !Abc "xyz"
			Abc <- "a" "b" "c"`,
		cases: []actionTestCase{
			{"xyz", []interface{}{true, "xyz"}},
		},
	},
	{
		name: "true pred code",
		grammar: `
			A <- &{ true } "abc"
			Abc <- "a" "b" "c"`,
		cases: []actionTestCase{
			{"abc", []interface{}{true, "abc"}},
		},
	},
	{
		name: "false pred code",
		grammar: `
			A <- !{ false } "xyz"
			Abc <- "a" "b" "c"`,
		cases: []actionTestCase{
			{"xyz", []interface{}{true, "xyz"}},
		},
	},
	{
		name:    "subexpr",
		grammar: `A <- ("a" "b" "c")`,
		cases: []actionTestCase{
			{"abc", []interface{}{"a", "b", "c"}},
		},
	},
	{
		name:    "label",
		grammar: `A <- l1:"a" l2:"b" l3:"c"`,
		cases: []actionTestCase{
			{"abc", []interface{}{"a", "b", "c"}},
		},
	},
	{
		name: "action",
		grammar: `
			A <- l1:. l2:. l3:. "map[string]string":{
				return map[string]string{
					"1": l1,
					"2": l2,
					"3": l3,
				}
			}`,
		cases: []actionTestCase{
			{"abc", map[string]interface{}{
				"1": "a",
				"2": "b",
				"3": "c",
			}},
			{"xyz", map[string]interface{}{
				"1": "x",
				"2": "y",
				"3": "z",
			}},
		},
	},

	// A simple calculator.
	// BUG: The test grammar has reverse the normal associativity — oops.
	{
		name: "calculator",
		grammar: `
			A <- Expr
			Expr <- l:Term op:(Plus / Minus) r:Expr int:{ return op(l, r) } / x:Term int:{ return x }
			Plus <- "+" "func(int, int) int":{ return func(a, b int) int { return a + b } }
			Minus <- "-" "func(int, int) int":{ return func(a, b int) int { return a - b } }
			Term <- l:Factor op:(Times / Divide) r:Term int:{ return op(l, r) } / x:Factor int:{ return x }
			Times <- "*" "func(int, int) int":{ return func(a, b int) int { return a * b } }
			Divide <- "/" "func(int, int) int":{ return func(a, b int) int { return a / b } }
			Factor <- Number / '(' x:Expr ')' int:{ return x }
			Number <- x:[0-9]+ int:{ var i int; for _, s := range x { i = i * 10 + (int(s[0]) - '0') }; return i }
		`,
		cases: []actionTestCase{
			{"1", 1.0},
			{"(5)", 5.0},
			{"2*3", 6.0},
			{"2+3", 5.0},
			{"10-3*2", 4.0},
			{"10-(6/2)*5", -5.0},
		},
	},
}

func TestActionGen(t *testing.T) {
	for _, test := range actionTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := genTest(actionPrelude, test.grammar)
			binary := build(source)
			defer rm(binary)
			go rm(source)

			for _, c := range test.cases {
				var got struct {
					T interface{}
				}
				parseJSON(binary, c.input, &got)
				if !reflect.DeepEqual(got.T, c.want) {
					t.Errorf("parse(%q)=%s (%#v), want %s",
						c.input, pretty.String(got.T), got.T,
						pretty.String(c.want))
				}
			}

		})
	}
}

// parseJSON parses an input using the given binary
// and returns the position of either the parse or error
// along with whether the parse succeeded.
// The format for transmitting the result
// from the parser binary to the test harness
// is JSON.
func parseJSON(binary, input string, result interface{}) {
	cmd := exec.Command(binary)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err.Error())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err.Error())
	}
	if err := cmd.Start(); err != nil {
		panic(err.Error())
	}
	go func() {
		if _, err := io.WriteString(stdin, input); err != nil {
			panic(err.Error())
		}
		if err := stdin.Close(); err != nil {
			panic(err.Error())
		}
	}()
	if err := json.NewDecoder(stdout).Decode(result); err != nil {
		panic(err.Error())
	}
	if err := cmd.Wait(); err != nil {
		panic(err.Error())
	}
}

var actionPrelude = `{
package main

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/eaburns/peggy/peg"
)

func main() {
	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	p := _NewParser(string(data))
	if pos, _ := _AAccepts(p, 0); pos < 0 {
		os.Stderr.WriteString("parse failed")
		os.Exit(1)
	}
	var result struct {
		T interface{}
	}
	_, result.T = _AAction(p, 0)
	if err := json.NewEncoder(os.Stdout).Encode(&result); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
}
`
