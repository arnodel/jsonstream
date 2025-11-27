package token

import (
	"math"
	"testing"
)

// TestStringScalar tests creation of string scalars
func TestStringScalar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", `""`},
		{"simple string", "hello", `"hello"`},
		{"string with spaces", "hello world", `"hello world"`},
		{"unicode string", "hello 世界", `"hello 世界"`},
		{"string with special chars", "tab\there", "\"tab\\there\""},
		{"string with quotes", `say "hello"`, `"say \"hello\""`},
		{"string with backslash", `path\to\file`, `"path\\to\\file"`},
		{"string with newline", "line1\nline2", `"line1\nline2"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := StringScalar(tt.input)
			if scalar.Type() != String {
				t.Errorf("expected type String, got %v", scalar.Type())
			}
			result := string(scalar.Bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestFloat64Scalar tests creation of float scalars
func TestFloat64Scalar(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"zero", 0.0, "0"},
		{"negative zero", -0.0, "0"},
		{"positive integer as float", 42.0, "42"},
		{"negative integer as float", -42.0, "-42"},
		{"simple decimal", 3.14, "3.14"},
		{"negative decimal", -3.14, "-3.14"},
		{"very small number", 0.0000001, "1e-07"},
		{"very large number", 1e20, "1e+20"},
		{"scientific notation", 1.5e10, "1.5e+10"},
		{"negative scientific", -1.5e10, "-1.5e+10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := Float64Scalar(tt.input)
			if scalar.Type() != Number {
				t.Errorf("expected type Number, got %v", scalar.Type())
			}
			result := string(scalar.Bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestFloat64ScalarSpecialValues tests special float values
func TestFloat64ScalarSpecialValues(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"NaN", math.NaN(), "NaN"},
		{"positive infinity", math.Inf(1), "+Inf"},
		{"negative infinity", math.Inf(-1), "-Inf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := Float64Scalar(tt.input)
			if scalar.Type() != Number {
				t.Errorf("expected type Number, got %v", scalar.Type())
			}
			result := string(scalar.Bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestInt64Scalar tests creation of integer scalars
func TestInt64Scalar(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"positive small", 42, "42"},
		{"negative small", -42, "-42"},
		{"max int64", math.MaxInt64, "9223372036854775807"},
		{"min int64", math.MinInt64, "-9223372036854775808"},
		{"one", 1, "1"},
		{"negative one", -1, "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := Int64Scalar(tt.input)
			if scalar.Type() != Number {
				t.Errorf("expected type Number, got %v", scalar.Type())
			}
			result := string(scalar.Bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestBoolScalar tests creation of boolean scalars
func TestBoolScalar(t *testing.T) {
	trueScalar := BoolScalar(true)
	if trueScalar.Type() != Boolean {
		t.Errorf("expected type Boolean, got %v", trueScalar.Type())
	}
	if string(trueScalar.Bytes) != "true" {
		t.Errorf("expected %q, got %q", "true", string(trueScalar.Bytes))
	}

	falseScalar := BoolScalar(false)
	if falseScalar.Type() != Boolean {
		t.Errorf("expected type Boolean, got %v", falseScalar.Type())
	}
	if string(falseScalar.Bytes) != "false" {
		t.Errorf("expected %q, got %q", "false", string(falseScalar.Bytes))
	}
}

// TestToScalar tests conversion of Go values to scalars
func TestToScalar(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expectType  ScalarType
		expectBytes string
		expectError bool
	}{
		{"nil", nil, Null, "null", false},
		{"string", "hello", String, `"hello"`, false},
		{"int", 42, Number, "42", false},
		{"int64", int64(42), Number, "42", false},
		{"float64", float64(3.14), Number, "3.14", false},
		{"bool true", true, Boolean, "true", false},
		{"bool false", false, Boolean, "false", false},
		// Unsupported numeric types
		{"int8", int8(42), 0, "", true},
		{"int16", int16(42), 0, "", true},
		{"int32", int32(42), 0, "", true},
		{"uint", uint(42), 0, "", true},
		{"uint8", uint8(42), 0, "", true},
		{"uint16", uint16(42), 0, "", true},
		{"uint32", uint32(42), 0, "", true},
		{"uint64", uint64(42), 0, "", true},
		{"float32", float32(3.14), 0, "", true},
		// Other unsupported types
		{"unsupported type - slice", []int{1, 2, 3}, 0, "", true},
		{"unsupported type - map", map[string]int{"a": 1}, 0, "", true},
		{"unsupported type - struct", struct{ X int }{42}, 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar, err := ToScalar(tt.input)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if scalar.Type() != tt.expectType {
				t.Errorf("expected type %v, got %v", tt.expectType, scalar.Type())
			}
			if tt.expectBytes != "" && string(scalar.Bytes) != tt.expectBytes {
				t.Errorf("expected bytes %q, got %q", tt.expectBytes, string(scalar.Bytes))
			}
		})
	}
}

// TestScalarEqual tests the Equal method with various combinations
func TestScalarEqual(t *testing.T) {
	tests := []struct {
		name     string
		s1       *Scalar
		s2       *Scalar
		expected bool
	}{
		{
			"identical strings",
			StringScalar("hello"),
			StringScalar("hello"),
			true,
		},
		{
			"different strings",
			StringScalar("hello"),
			StringScalar("world"),
			false,
		},
		{
			"identical numbers",
			Int64Scalar(42),
			Int64Scalar(42),
			true,
		},
		{
			"different numbers",
			Int64Scalar(42),
			Int64Scalar(43),
			false,
		},
		{
			"same number different format",
			Float64Scalar(42.0),
			Int64Scalar(42),
			true,
		},
		{
			"true booleans",
			BoolScalar(true),
			BoolScalar(true),
			true,
		},
		{
			"false booleans",
			BoolScalar(false),
			BoolScalar(false),
			true,
		},
		{
			"different booleans",
			BoolScalar(true),
			BoolScalar(false),
			false,
		},
		{
			"null values",
			NullScalar,
			NullScalar,
			true,
		},
		{
			"string vs number",
			StringScalar("42"),
			Int64Scalar(42),
			false,
		},
		{
			"string vs boolean",
			StringScalar("true"),
			BoolScalar(true),
			false,
		},
		{
			"number vs boolean",
			Int64Scalar(1),
			BoolScalar(true),
			false,
		},
		{
			"null vs string",
			NullScalar,
			StringScalar("null"),
			false,
		},
		{
			"empty strings",
			StringScalar(""),
			StringScalar(""),
			true,
		},
		{
			"zero vs negative zero",
			Float64Scalar(0.0),
			Float64Scalar(-0.0),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.s1.Equal(tt.s2)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
			// Test symmetry
			result2 := tt.s2.Equal(tt.s1)
			if result2 != result {
				t.Errorf("Equal is not symmetric: s1.Equal(s2)=%v but s2.Equal(s1)=%v", result, result2)
			}
		})
	}
}

// TestScalarToString tests the ToString method
func TestScalarToString(t *testing.T) {
	tests := []struct {
		name     string
		scalar   *Scalar
		expected string
	}{
		{"simple string", StringScalar("hello"), "hello"},
		{"empty string", StringScalar(""), ""},
		{"string with spaces", StringScalar("hello world"), "hello world"},
		{"unicode", StringScalar("世界"), "世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.scalar.ToString()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestScalarToStringPanic tests that ToString panics on non-strings
func TestScalarToStringPanic(t *testing.T) {
	tests := []struct {
		name   string
		scalar *Scalar
	}{
		{"number", Int64Scalar(42)},
		{"boolean", BoolScalar(true)},
		{"null", NullScalar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic, got none")
				}
			}()
			tt.scalar.ToString()
		})
	}
}

// TestScalarToGo tests the ToGo method
func TestScalarToGo(t *testing.T) {
	tests := []struct {
		name     string
		scalar   *Scalar
		expected any
	}{
		{"string", StringScalar("hello"), "hello"},
		{"number int", Int64Scalar(42), float64(42)},
		{"number float", Float64Scalar(3.14), 3.14},
		{"boolean true", BoolScalar(true), true},
		{"boolean false", BoolScalar(false), false},
		{"null", NullScalar, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.scalar.ToGo()
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

// TestScalarEqualsString tests the EqualsString method
func TestScalarEqualsString(t *testing.T) {
	tests := []struct {
		name     string
		scalar   *Scalar
		str      string
		expected bool
	}{
		{"matching string", StringScalar("hello"), "hello", true},
		{"different string", StringScalar("hello"), "world", false},
		{"empty string match", StringScalar(""), "", true},
		// EqualsString only works for String type scalars
		{"number returns false", Int64Scalar(42), "42", false},
		{"boolean returns false", BoolScalar(true), "true", false},
		{"null returns false", NullScalar, "null", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.scalar.EqualsString(tt.str)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestScalarIsKey tests the IsKey method
func TestScalarIsKey(t *testing.T) {
	// Create a key scalar
	keyScalar := StringScalar("mykey")
	keyScalar.TypeAndFlags |= KeyMask

	// Create a non-key scalar
	nonKeyScalar := StringScalar("mykey")

	if !keyScalar.IsKey() {
		t.Error("expected IsKey to return true for key scalar")
	}

	if nonKeyScalar.IsKey() {
		t.Error("expected IsKey to return false for non-key scalar")
	}
}

// TestScalarIsAlnum tests the IsAlnum method
func TestScalarIsAlnum(t *testing.T) {
	// Create an alphanumeric scalar
	alnumScalar := StringScalar("field_name_123")
	alnumScalar.TypeAndFlags |= AlnumMask

	// Create a non-alphanumeric scalar
	nonAlnumScalar := StringScalar("field-name")

	if !alnumScalar.IsAlnum() {
		t.Error("expected IsAlnum to return true for alnum scalar")
	}

	if nonAlnumScalar.IsAlnum() {
		t.Error("expected IsAlnum to return false for non-alnum scalar")
	}
}

// TestScalarIsUnescaped tests the IsUnescaped method
func TestScalarIsUnescaped(t *testing.T) {
	// Create an unescaped scalar
	unescapedScalar := StringScalar("simple")
	unescapedScalar.TypeAndFlags |= UnescapedMask

	// Create an escaped scalar
	escapedScalar := StringScalar("with\"quotes")

	if !unescapedScalar.IsUnescaped() {
		t.Error("expected IsUnescaped to return true for unescaped scalar")
	}

	if escapedScalar.IsUnescaped() {
		t.Error("expected IsUnescaped to return false for escaped scalar")
	}
}

// TestScalarType tests the Type method
func TestScalarType(t *testing.T) {
	tests := []struct {
		name         string
		scalar       *Scalar
		expectedType ScalarType
	}{
		{"string", StringScalar("hello"), String},
		{"number int", Int64Scalar(42), Number},
		{"number float", Float64Scalar(3.14), Number},
		{"boolean true", BoolScalar(true), Boolean},
		{"boolean false", BoolScalar(false), Boolean},
		{"null", NullScalar, Null},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.scalar.Type()
			if result != tt.expectedType {
				t.Errorf("expected type %v, got %v", tt.expectedType, result)
			}
		})
	}
}

// TestNewKey tests the NewKey function
func TestNewKey(t *testing.T) {
	key := NewKey(String, []byte(`"mykey"`))

	if key.Type() != String {
		t.Errorf("expected type String, got %v", key.Type())
	}

	if !key.IsKey() {
		t.Error("expected IsKey to return true")
	}

	if string(key.Bytes) != `"mykey"` {
		t.Errorf("expected bytes %q, got %q", `"mykey"`, string(key.Bytes))
	}
}

// TestNewScalar tests the NewScalar function
func TestNewScalar(t *testing.T) {
	scalar := NewScalar(String, []byte(`"hello"`))

	if scalar.Type() != String {
		t.Errorf("expected type String, got %v", scalar.Type())
	}

	if string(scalar.Bytes) != `"hello"` {
		t.Errorf("expected bytes %q, got %q", `"hello"`, string(scalar.Bytes))
	}
}

// TestScalarConstants tests the predefined scalar constants
func TestScalarConstants(t *testing.T) {
	// Test TrueScalar
	if TrueScalar.Type() != Boolean {
		t.Errorf("TrueScalar: expected type Boolean, got %v", TrueScalar.Type())
	}
	if string(TrueScalar.Bytes) != "true" {
		t.Errorf("TrueScalar: expected bytes %q, got %q", "true", string(TrueScalar.Bytes))
	}

	// Test FalseScalar
	if FalseScalar.Type() != Boolean {
		t.Errorf("FalseScalar: expected type Boolean, got %v", FalseScalar.Type())
	}
	if string(FalseScalar.Bytes) != "false" {
		t.Errorf("FalseScalar: expected bytes %q, got %q", "false", string(FalseScalar.Bytes))
	}

	// Test NullScalar
	if NullScalar.Type() != Null {
		t.Errorf("NullScalar: expected type Null, got %v", NullScalar.Type())
	}
	if string(NullScalar.Bytes) != "null" {
		t.Errorf("NullScalar: expected bytes %q, got %q", "null", string(NullScalar.Bytes))
	}
}

// TestTokenStringMethods tests the String() methods on token types
func TestTokenStringMethods(t *testing.T) {
	tests := []struct {
		name     string
		token    Token
		expected string
	}{
		{"StartObject", &StartObject{}, "StartObject"},
		{"EndObject", &EndObject{}, "EndObject"},
		{"StartArray", &StartArray{}, "StartArray"},
		{"EndArray", &EndArray{}, "EndArray"},
		{"Elision", &Elision{}, "Elision"},
		{"Scalar string", StringScalar("hello"), `Scalar("hello")`},
		{"Scalar number", Int64Scalar(42), "Scalar(42)"},
		{"Scalar boolean", BoolScalar(true), "Scalar(true)"},
		{"Scalar null", NullScalar, "Scalar(null)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.String()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestScalarEqualEdgeCases tests additional edge cases for Equal method
func TestScalarEqualEdgeCases(t *testing.T) {
	// Test escaped vs unescaped strings
	escaped := StringScalar("hello\nworld")
	unescaped := NewScalar(String, []byte(`"simple"`))
	unescaped.TypeAndFlags |= UnescapedMask

	// They should be equal if they represent the same string value
	if escaped.Equal(escaped) != true {
		t.Error("scalar should equal itself")
	}
	if unescaped.Equal(unescaped) != true {
		t.Error("scalar should equal itself")
	}

	// Test numbers in different formats that represent the same value
	num1 := Float64Scalar(1.0)
	num2 := Int64Scalar(1)
	if !num1.Equal(num2) {
		t.Error("1.0 should equal 1")
	}
}

// TestScalarStringScalarEncoding tests edge cases in StringScalar encoding
func TestScalarStringScalarEncoding(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"control characters", "hello\t\n\r\fworld"},
		{"unicode emoji", "hello 👋 world"},
		{"unicode CJK", "你好世界"},
		{"mixed escaping", `path\to\file with "quotes"`},
		{"forward slash", "path/to/file"},
		{"all quotes", `"""""""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := StringScalar(tt.input)

			// Verify it round-trips correctly
			result := scalar.ToString()
			if result != tt.input {
				t.Errorf("round-trip failed: expected %q, got %q", tt.input, result)
			}
		})
	}
}

// TestScalarToGoEdgeCases tests edge cases for ToGo conversion
func TestScalarToGoEdgeCases(t *testing.T) {
	// Test that unescaped strings work
	unescaped := NewScalar(String, []byte(`"simple"`))
	unescaped.TypeAndFlags |= UnescapedMask

	result := unescaped.ToGo()
	if result != "simple" {
		t.Errorf("expected %q, got %v", "simple", result)
	}

	// Test empty string
	empty := StringScalar("")
	result = empty.ToGo()
	if result != "" {
		t.Errorf("expected empty string, got %v", result)
	}
}
