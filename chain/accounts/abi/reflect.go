package abi

import (
	"fmt"
	"reflect"
	"strings"
)

func indirect(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Ptr && v.Elem().Type() != derefbigT {
		return indirect(v.Elem())
	}
	return v
}

func indirectInterfaceOrPtr(v reflect.Value) reflect.Value {
	if (v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr) && v.Elem().IsValid() {
		return indirect(v.Elem())
	}
	return v
}

func reflectIntKindAndType(unsigned bool, size int) (reflect.Kind, reflect.Type) {
	switch size {
	case 8:
		if unsigned {
			return reflect.Uint8, uint8T
		}
		return reflect.Int8, int8T
	case 16:
		if unsigned {
			return reflect.Uint16, uint16T
		}
		return reflect.Int16, int16T
	case 32:
		if unsigned {
			return reflect.Uint32, uint32T
		}
		return reflect.Int32, int32T
	case 64:
		if unsigned {
			return reflect.Uint64, uint64T
		}
		return reflect.Int64, int64T
	}
	return reflect.Ptr, bigT
}

func mustArrayToByteSlice(value reflect.Value) reflect.Value {
	slice := reflect.MakeSlice(reflect.TypeOf([]byte{}), value.Len(), value.Len())
	reflect.Copy(slice, value)
	return slice
}

func set(dst, src reflect.Value) error {
	dstType, srcType := dst.Type(), src.Type()
	switch {
	case dstType.Kind() == reflect.Interface && dst.Elem().IsValid():
		return set(dst.Elem(), src)
	case dstType.Kind() == reflect.Ptr && dstType.Elem() != derefbigT:
		return set(dst.Elem(), src)
	case srcType.AssignableTo(dstType) && dst.CanSet():
		dst.Set(src)
	case dstType.Kind() == reflect.Slice && srcType.Kind() == reflect.Slice:
		return setSlice(dst, src)
	default:
		return fmt.Errorf("abi: cannot unmarshal %v in to %v", src.Type(), dst.Type())
	}
	return nil
}

func setSlice(dst, src reflect.Value) error {
	slice := reflect.MakeSlice(dst.Type(), src.Len(), src.Len())
	for i := 0; i < src.Len(); i++ {
		v := src.Index(i)
		reflect.Copy(slice.Index(i), v)
	}

	dst.Set(slice)
	return nil
}

func requireAssignable(dst, src reflect.Value) error {
	if dst.Kind() != reflect.Ptr && dst.Kind() != reflect.Interface {
		return fmt.Errorf("abi: cannot unmarshal %v into %v", src.Type(), dst.Type())
	}
	return nil
}

func requireUnpackKind(v reflect.Value, t reflect.Type, k reflect.Kind,
	args Arguments) error {

	switch k {
	case reflect.Struct:
	case reflect.Slice, reflect.Array:
		if minLen := args.LengthNonIndexed(); v.Len() < minLen {
			return fmt.Errorf("abi: insufficient number of elements in the list/array for unpack, want %d, got %d",
				minLen, v.Len())
		}
	default:
		return fmt.Errorf("abi: cannot unmarshal tuple into %v", t)
	}
	return nil
}

func mapArgNamesToStructFields(argNames []string, value reflect.Value) (map[string]string, error) {
	typ := value.Type()

	abi2struct := make(map[string]string)
	struct2abi := make(map[string]string)

	for i := 0; i < typ.NumField(); i++ {
		structFieldName := typ.Field(i).Name

		if structFieldName[:1] != strings.ToUpper(structFieldName[:1]) {
			continue
		}

		var ok bool
		var tagName string
		if tagName, ok = typ.Field(i).Tag.Lookup("abi"); !ok {
			continue
		}

		if tagName == "" {
			return nil, fmt.Errorf("struct: abi tag in '%s' is empty", structFieldName)
		}

		found := false
		for _, arg := range argNames {
			if arg == tagName {
				if abi2struct[arg] != "" {
					return nil, fmt.Errorf("struct: abi tag in '%s' already mapped", structFieldName)
				}

				abi2struct[arg] = structFieldName
				struct2abi[structFieldName] = arg
				found = true
			}
		}

		if !found {
			return nil, fmt.Errorf("struct: abi tag '%s' defined but not found in abi", tagName)
		}
	}

	for _, argName := range argNames {

		structFieldName := ToCamelCase(argName)

		if structFieldName == "" {
			return nil, fmt.Errorf("abi: purely underscored output cannot unpack to struct")
		}

		if abi2struct[argName] != "" {
			if abi2struct[argName] != structFieldName &&
				struct2abi[structFieldName] == "" &&
				value.FieldByName(structFieldName).IsValid() {
				return nil, fmt.Errorf("abi: multiple variables maps to the same abi field '%s'", argName)
			}
			continue
		}

		if struct2abi[structFieldName] != "" {
			return nil, fmt.Errorf("abi: multiple outputs mapping to the same struct field '%s'", structFieldName)
		}

		if value.FieldByName(structFieldName).IsValid() {

			abi2struct[argName] = structFieldName
			struct2abi[structFieldName] = argName
		} else {

			struct2abi[structFieldName] = argName
		}
	}
	return abi2struct, nil
}
