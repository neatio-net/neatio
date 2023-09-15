package bind

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/nio-net/nio/chain/accounts/abi"
	"github.com/nio-net/nio/chain/log"
)

type Lang int

const (
	LangGo Lang = iota
	LangJava
	LangObjC
)

func Bind(types []string, abis []string, bytecodes []string, fsigs []map[string]string, pkg string, lang Lang, libs map[string]string, aliases map[string]string) (string, error) {
	var (
		contracts = make(map[string]*tmplContract)

		structs = make(map[string]*tmplStruct)

		isLib = make(map[string]struct{})
	)
	for i := 0; i < len(types); i++ {

		evmABI, err := abi.JSON(strings.NewReader(abis[i]))
		if err != nil {
			return "", err
		}

		strippedABI := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, abis[i])

		var (
			calls     = make(map[string]*tmplMethod)
			transacts = make(map[string]*tmplMethod)
			events    = make(map[string]*tmplEvent)

			callIdentifiers     = make(map[string]bool)
			transactIdentifiers = make(map[string]bool)
			eventIdentifiers    = make(map[string]bool)
		)
		for _, original := range evmABI.Methods {

			normalized := original
			normalizedName := methodNormalizer[lang](alias(aliases, original.Name))

			var identifiers = callIdentifiers
			if !original.Const {
				identifiers = transactIdentifiers
			}
			if identifiers[normalizedName] {
				return "", fmt.Errorf("duplicated identifier \"%s\"(normalized \"%s\"), use --alias for renaming", original.Name, normalizedName)
			}
			identifiers[normalizedName] = true
			normalized.Name = normalizedName
			normalized.Inputs = make([]abi.Argument, len(original.Inputs))
			copy(normalized.Inputs, original.Inputs)
			for j, input := range normalized.Inputs {
				if input.Name == "" {
					normalized.Inputs[j].Name = fmt.Sprintf("arg%d", j)
				}
				if hasStruct(input.Type) {
					bindStructType[lang](input.Type, structs)
				}
			}
			normalized.Outputs = make([]abi.Argument, len(original.Outputs))
			copy(normalized.Outputs, original.Outputs)
			for j, output := range normalized.Outputs {
				if output.Name != "" {
					normalized.Outputs[j].Name = capitalise(output.Name)
				}
				if hasStruct(output.Type) {
					bindStructType[lang](output.Type, structs)
				}
			}

			if original.Const {
				calls[original.Name] = &tmplMethod{Original: original, Normalized: normalized, Structured: structured(original.Outputs)}
			} else {
				transacts[original.Name] = &tmplMethod{Original: original, Normalized: normalized, Structured: structured(original.Outputs)}
			}
		}
		for _, original := range evmABI.Events {

			if original.Anonymous {
				continue
			}

			normalized := original

			normalizedName := methodNormalizer[lang](alias(aliases, original.Name))
			if eventIdentifiers[normalizedName] {
				return "", fmt.Errorf("duplicated identifier \"%s\"(normalized \"%s\"), use --alias for renaming", original.Name, normalizedName)
			}
			eventIdentifiers[normalizedName] = true
			normalized.Name = normalizedName

			normalized.Inputs = make([]abi.Argument, len(original.Inputs))
			copy(normalized.Inputs, original.Inputs)
			for j, input := range normalized.Inputs {
				if input.Name == "" {
					normalized.Inputs[j].Name = fmt.Sprintf("arg%d", j)
				}
				if hasStruct(input.Type) {
					bindStructType[lang](input.Type, structs)
				}
			}

			events[original.Name] = &tmplEvent{Original: original, Normalized: normalized}
		}

		if len(structs) > 0 && lang == LangJava {
			return "", errors.New("java binding for tuple arguments is not supported yet")
		}

		contracts[types[i]] = &tmplContract{
			Type:        capitalise(types[i]),
			InputABI:    strings.Replace(strippedABI, "\"", "\\\"", -1),
			InputBin:    strings.TrimPrefix(strings.TrimSpace(bytecodes[i]), "0x"),
			Constructor: evmABI.Constructor,
			Calls:       calls,
			Transacts:   transacts,
			Events:      events,
			Libraries:   make(map[string]string),
		}

		if len(fsigs) > i {
			contracts[types[i]].FuncSigs = fsigs[i]
		}

		for pattern, name := range libs {
			matched, err := regexp.Match("__\\$"+pattern+"\\$__", []byte(contracts[types[i]].InputBin))
			if err != nil {
				log.Error("Could not search for pattern", "pattern", pattern, "contract", contracts[types[i]], "err", err)
			}
			if matched {
				contracts[types[i]].Libraries[pattern] = name

				if _, ok := isLib[name]; !ok {
					isLib[name] = struct{}{}
				}
			}
		}
	}

	for i := 0; i < len(types); i++ {
		_, ok := isLib[types[i]]
		contracts[types[i]].Library = ok
	}

	data := &tmplData{
		Package:   pkg,
		Contracts: contracts,
		Libraries: libs,
		Structs:   structs,
	}
	buffer := new(bytes.Buffer)

	funcs := map[string]interface{}{
		"bindtype":      bindType[lang],
		"bindtopictype": bindTopicType[lang],
		"namedtype":     namedType[lang],
		"formatmethod":  formatMethod,
		"formatevent":   formatEvent,
		"capitalise":    capitalise,
		"decapitalise":  decapitalise,
	}
	tmpl := template.Must(template.New("").Funcs(funcs).Parse(tmplSource[lang]))
	if err := tmpl.Execute(buffer, data); err != nil {
		return "", err
	}

	if lang == LangGo {
		code, err := format.Source(buffer.Bytes())
		if err != nil {
			return "", fmt.Errorf("%v\n%s", err, buffer)
		}
		return string(code), nil
	}

	return buffer.String(), nil
}

var bindType = map[Lang]func(kind abi.Type, structs map[string]*tmplStruct) string{
	LangGo:   bindTypeGo,
	LangJava: bindTypeJava,
}

func bindBasicTypeGo(kind abi.Type) string {
	switch kind.T {
	case abi.AddressTy:
		return "common.Address"
	case abi.IntTy, abi.UintTy:
		parts := regexp.MustCompile(`(u)?int([0-9]*)`).FindStringSubmatch(kind.String())
		switch parts[2] {
		case "8", "16", "32", "64":
			return fmt.Sprintf("%sint%s", parts[1], parts[2])
		}
		return "*big.Int"
	case abi.FixedBytesTy:
		return fmt.Sprintf("[%d]byte", kind.Size)
	case abi.BytesTy:
		return "[]byte"
	case abi.FunctionTy:
		return "[24]byte"
	default:

		return kind.String()
	}
}

func bindTypeGo(kind abi.Type, structs map[string]*tmplStruct) string {
	switch kind.T {
	case abi.TupleTy:
		return structs[kind.TupleRawName+kind.String()].Name
	case abi.ArrayTy:
		return fmt.Sprintf("[%d]", kind.Size) + bindTypeGo(*kind.Elem, structs)
	case abi.SliceTy:
		return "[]" + bindTypeGo(*kind.Elem, structs)
	default:
		return bindBasicTypeGo(kind)
	}
}

func bindBasicTypeJava(kind abi.Type) string {
	switch kind.T {
	case abi.AddressTy:
		return "Address"
	case abi.IntTy, abi.UintTy:

		parts := regexp.MustCompile(`(u)?int([0-9]*)`).FindStringSubmatch(kind.String())
		if len(parts) != 3 {
			return kind.String()
		}

		if parts[1] == "u" {
			return "BigInt"
		}

		namedSize := map[string]string{
			"8":  "byte",
			"16": "short",
			"32": "int",
			"64": "long",
		}[parts[2]]

		if namedSize == "" {
			namedSize = "BigInt"
		}
		return namedSize
	case abi.FixedBytesTy, abi.BytesTy:
		return "byte[]"
	case abi.BoolTy:
		return "boolean"
	case abi.StringTy:
		return "String"
	case abi.FunctionTy:
		return "byte[24]"
	default:
		return kind.String()
	}
}

func pluralizeJavaType(typ string) string {
	switch typ {
	case "boolean":
		return "Bools"
	case "String":
		return "Strings"
	case "Address":
		return "Addresses"
	case "byte[]":
		return "Binaries"
	case "BigInt":
		return "BigInts"
	}
	return typ + "[]"
}

func bindTypeJava(kind abi.Type, structs map[string]*tmplStruct) string {
	switch kind.T {
	case abi.TupleTy:
		return structs[kind.TupleRawName+kind.String()].Name
	case abi.ArrayTy, abi.SliceTy:
		return pluralizeJavaType(bindTypeJava(*kind.Elem, structs))
	default:
		return bindBasicTypeJava(kind)
	}
}

var bindTopicType = map[Lang]func(kind abi.Type, structs map[string]*tmplStruct) string{
	LangGo:   bindTopicTypeGo,
	LangJava: bindTopicTypeJava,
}

func bindTopicTypeGo(kind abi.Type, structs map[string]*tmplStruct) string {
	bound := bindTypeGo(kind, structs)

	if bound == "string" || bound == "[]byte" {
		bound = "common.Hash"
	}
	return bound
}

func bindTopicTypeJava(kind abi.Type, structs map[string]*tmplStruct) string {
	bound := bindTypeJava(kind, structs)

	if bound == "String" || bound == "byte[]" {
		bound = "Hash"
	}
	return bound
}

var bindStructType = map[Lang]func(kind abi.Type, structs map[string]*tmplStruct) string{
	LangGo:   bindStructTypeGo,
	LangJava: bindStructTypeJava,
}

func bindStructTypeGo(kind abi.Type, structs map[string]*tmplStruct) string {
	switch kind.T {
	case abi.TupleTy:

		id := kind.TupleRawName + kind.String()
		if s, exist := structs[id]; exist {
			return s.Name
		}
		var fields []*tmplField
		for i, elem := range kind.TupleElems {
			field := bindStructTypeGo(*elem, structs)
			fields = append(fields, &tmplField{Type: field, Name: capitalise(kind.TupleRawNames[i]), SolKind: *elem})
		}
		name := kind.TupleRawName
		if name == "" {
			name = fmt.Sprintf("Struct%d", len(structs))
		}
		structs[id] = &tmplStruct{
			Name:   name,
			Fields: fields,
		}
		return name
	case abi.ArrayTy:
		return fmt.Sprintf("[%d]", kind.Size) + bindStructTypeGo(*kind.Elem, structs)
	case abi.SliceTy:
		return "[]" + bindStructTypeGo(*kind.Elem, structs)
	default:
		return bindBasicTypeGo(kind)
	}
}

func bindStructTypeJava(kind abi.Type, structs map[string]*tmplStruct) string {
	switch kind.T {
	case abi.TupleTy:

		id := kind.TupleRawName + kind.String()
		if s, exist := structs[id]; exist {
			return s.Name
		}
		var fields []*tmplField
		for i, elem := range kind.TupleElems {
			field := bindStructTypeJava(*elem, structs)
			fields = append(fields, &tmplField{Type: field, Name: decapitalise(kind.TupleRawNames[i]), SolKind: *elem})
		}
		name := kind.TupleRawName
		if name == "" {
			name = fmt.Sprintf("Class%d", len(structs))
		}
		structs[id] = &tmplStruct{
			Name:   name,
			Fields: fields,
		}
		return name
	case abi.ArrayTy, abi.SliceTy:
		return pluralizeJavaType(bindStructTypeJava(*kind.Elem, structs))
	default:
		return bindBasicTypeJava(kind)
	}
}

var namedType = map[Lang]func(string, abi.Type) string{
	LangGo:   func(string, abi.Type) string { panic("this shouldn't be needed") },
	LangJava: namedTypeJava,
}

func namedTypeJava(javaKind string, solKind abi.Type) string {
	switch javaKind {
	case "byte[]":
		return "Binary"
	case "boolean":
		return "Bool"
	default:
		parts := regexp.MustCompile(`(u)?int([0-9]*)(\[[0-9]*\])?`).FindStringSubmatch(solKind.String())
		if len(parts) != 4 {
			return javaKind
		}
		switch parts[2] {
		case "8", "16", "32", "64":
			if parts[3] == "" {
				return capitalise(fmt.Sprintf("%sint%s", parts[1], parts[2]))
			}
			return capitalise(fmt.Sprintf("%sint%ss", parts[1], parts[2]))

		default:
			return javaKind
		}
	}
}

func alias(aliases map[string]string, n string) string {
	if alias, exist := aliases[n]; exist {
		return alias
	}
	return n
}

var methodNormalizer = map[Lang]func(string) string{
	LangGo:   abi.ToCamelCase,
	LangJava: decapitalise,
}

func capitalise(input string) string {
	return abi.ToCamelCase(input)
}

func decapitalise(input string) string {
	if len(input) == 0 {
		return input
	}

	goForm := abi.ToCamelCase(input)
	return strings.ToLower(goForm[:1]) + goForm[1:]
}

func structured(args abi.Arguments) bool {
	if len(args) < 2 {
		return false
	}
	exists := make(map[string]bool)
	for _, out := range args {

		if out.Name == "" {
			return false
		}

		field := capitalise(out.Name)
		if field == "" || exists[field] {
			return false
		}
		exists[field] = true
	}
	return true
}

func hasStruct(t abi.Type) bool {
	switch t.T {
	case abi.SliceTy:
		return hasStruct(*t.Elem)
	case abi.ArrayTy:
		return hasStruct(*t.Elem)
	case abi.TupleTy:
		return true
	default:
		return false
	}
}

func resolveArgName(arg abi.Argument, structs map[string]*tmplStruct) string {
	var (
		prefix   string
		embedded string
		typ      = &arg.Type
	)
loop:
	for {
		switch typ.T {
		case abi.SliceTy:
			prefix += "[]"
		case abi.ArrayTy:
			prefix += fmt.Sprintf("[%d]", typ.Size)
		default:
			embedded = typ.TupleRawName + typ.String()
			break loop
		}
		typ = typ.Elem
	}
	if s, exist := structs[embedded]; exist {
		return prefix + s.Name
	} else {
		return arg.Type.String()
	}
}

func formatMethod(method abi.Method, structs map[string]*tmplStruct) string {
	inputs := make([]string, len(method.Inputs))
	for i, input := range method.Inputs {
		inputs[i] = fmt.Sprintf("%v %v", resolveArgName(input, structs), input.Name)
	}
	outputs := make([]string, len(method.Outputs))
	for i, output := range method.Outputs {
		outputs[i] = resolveArgName(output, structs)
		if len(output.Name) > 0 {
			outputs[i] += fmt.Sprintf(" %v", output.Name)
		}
	}
	constant := ""
	if method.Const {
		constant = "constant "
	}
	return fmt.Sprintf("function %v(%v) %sreturns(%v)", method.RawName, strings.Join(inputs, ", "), constant, strings.Join(outputs, ", "))
}

func formatEvent(event abi.Event, structs map[string]*tmplStruct) string {
	inputs := make([]string, len(event.Inputs))
	for i, input := range event.Inputs {
		if input.Indexed {
			inputs[i] = fmt.Sprintf("%v indexed %v", resolveArgName(input, structs), input.Name)
		} else {
			inputs[i] = fmt.Sprintf("%v %v", resolveArgName(input, structs), input.Name)
		}
	}
	return fmt.Sprintf("event %v(%v)", event.RawName, strings.Join(inputs, ", "))
}
