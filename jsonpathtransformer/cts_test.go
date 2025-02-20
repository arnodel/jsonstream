package jsonpathtransformer_test

import (
	"errors"
	"os"
	"testing"

	"github.com/arnodel/jsonstream"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/jsonpathtransformer"
	"github.com/arnodel/jsonstream/token"
)

func TestRunCTS(t *testing.T) {
	ctsFile, err := os.Open("cts.json")
	if err != nil {
		t.Fatalf("Problem with CTS file: %s", err)
	}
	decoder := jsonstream.NewJSONDecoder(ctsFile)
	stream := token.ChannelReadStream(token.StartStream(decoder, nil))
	iter := iterator.New(stream)
	if !iter.Advance() {
		t.Fatalf("Expected an item")
	}
	ctsObj, ok := iter.CurrentValue().(*iterator.Object)
	if !ok {
		t.Fatal("Expected object")
	}
	for ctsObj.Advance() {
		key, val := ctsObj.CurrentKeyVal()
		if key.EqualsString("tests") {
			runCTSTests(t, val)
		}
	}
}

type ctsData struct {
	name             string
	selector         string
	document         iterator.Value
	result           iterator.Value
	results          *iterator.Array
	invalid_selector bool
}

func runCTSTests(t *testing.T, tests iterator.Value) {
	testsArr, ok := tests.(*iterator.Array)
	if !ok {
		t.Fatal("Expected tests to be an array")
	}
	for testsArr.Advance() {
		runCTSTest(t, testsArr.CurrentValue())

	}
}

func runCTSTest(t *testing.T, test iterator.Value) {
	testObj, ok := test.(*iterator.Object)
	if !ok {
		t.Fatal("Expected test to be an object")
	}
	testData := ctsData{}
	for testObj.Advance() {
		key, value := testObj.CurrentKeyVal()
		switch key.ToString() {
		case "name":
			valueScalar, ok := value.AsScalar()
			if !ok {
				t.Fatalf("Expected name to be a string")
			}
			testData.name = valueScalar.ToString()
		case "document":
			doc, detach := value.Clone()
			if detach != nil {
				defer detach()
			}
			testData.document = doc
		case "selector":
			valueScalar, ok := value.AsScalar()
			if !ok {
				t.Fatalf("Expected selector to be a string")
			}
			testData.selector = valueScalar.ToString()
		case "result":
			res, detach := value.Clone()
			if detach != nil {
				defer detach()
			}
			testData.result = res
		case "results":
			res, detach := value.Clone()
			if detach != nil {
				defer detach()
			}
			resArray, ok := res.AsArray()
			if !ok {
				t.Fatalf("Expected results to be an array")
			}
			testData.results = resArray
		case "invalid_selector":
			valueScalar, ok := value.AsScalar()
			if !ok {
				t.Fatalf("Expected invalid_selector to be a boolean")
			}
			switch x := valueScalar.ToGo().(type) {
			case bool:
				testData.invalid_selector = x
			default:
				t.Fatalf("Expected invalid_selector to be a boolean")
			}
		}
	}
	if !testData.invalid_selector && testData.result == nil && testData.results == nil {
		t.Fatalf("invalid test: missing result or results (%s)", testData.name)
	}
	t.Run(testData.name, func(t *testing.T) {
		runCTSTestData(t, testData)
	})
}

func runCTSTestData(t *testing.T, testData ctsData) {
	runner, err := compileQueryStringStrict(testData.selector)
	if err != nil {
		if errors.Is(err, jsonpathtransformer.ErrUnimplementedFeature) {
			t.Skipf("unimplemented feature: %s", err)
		}
		if testData.invalid_selector {
			return
		}
		t.Fatalf("invalid query {%s}: %s", testData.selector, err)
	}
	if testData.invalid_selector {
		t.Fatalf("query expected to be invalid")
	}
	if testData.result != nil {
		expectedArr, ok := testData.result.AsArray()
		if !ok {
			t.Fatal("Expected result to be an array")
		}
		runner.EvaluateNodesResult(testData.document).ForEachNode(func(val iterator.Value) bool {
			if !expectedArr.Advance() {
				t.Fatal("got more nodes than expected in query result")
			}
			expectedVal := expectedArr.CurrentValue()
			if !iterator.ValuesEqual(val, expectedVal) {
				t.Fatal("non-matching results")
			}
			return true
		})
		if expectedArr.Advance() {
			t.Fatal("expected more nodes in query result")
		}
	}
	if testData.results != nil {
		for testData.results.Advance() {
			expectedArr, ok := testData.results.CurrentValue().AsArray()
			if !ok {
				t.Fatal("Expected results item to be an array")
			}
			document, detach := testData.document.Clone()
			match := true
			runner.EvaluateNodesResult(document).ForEachNode(func(val iterator.Value) bool {
				if !expectedArr.Advance() {
					match = false
					return false
				}
				expectedVal := expectedArr.CurrentValue()
				if !iterator.ValuesEqual(val, expectedVal) {
					match = false
					return false
				}
				return true
			})
			detach()
			if match && !expectedArr.Advance() {
				return
			}
		}
		t.Fatal("Result does not match any of the expected results")
	}
}
