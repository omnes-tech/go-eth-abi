package abi

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

func Parse(decoded []any, v any) error {
	return parseStruct(decoded, v)
}

// parseStruct parses decoded values into a struct
func parseStruct(decoded []any, structVal any) error {
	rv := reflect.ValueOf(structVal)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("[parseStruct] v must be a pointer")
	}

	rve := rv.Elem()
	if rve.Kind() != reflect.Struct {
		return fmt.Errorf("[parseStruct] v must be a struct pointer")
	}

	if len(decoded) != rve.NumField() && rve.Type().String() != "big.Int" && rve.Type().String() != "common.Address" {
		return fmt.Errorf("[parseStruct] number of decoded values does not match number of struct fields")
	}

	for i := 0; i < rve.NumField(); i++ {
		field := rve.Field(i)
		vType := reflect.TypeOf(decoded[i])
		if field.Kind() == reflect.Ptr && field.Type().Elem().String() != "big.Int" && field.Type().Elem().String() != "common.Address" {
			var err error
			if field.Type().Elem().Kind() == reflect.Struct {
				if field.IsNil() {
					field.Set(reflect.New(field.Type().Elem()))
				}
				err = parseStruct(decoded[i].([]any), field.Interface())
			} else {
				err = parsePointer([]any{decoded[i]}, field)
			}
			if err != nil {
				return fmt.Errorf("[parseStruct] error parsing pointer field %s: %w", field.Type().Name(), err)
			}
		} else if field.Kind() == reflect.Struct {
			err := parseStruct(decoded[i].([]any), field.Addr().Interface())
			if err != nil {
				return fmt.Errorf("[parseStruct] error parsing struct field %s: %w", field.Type().Name(), err)
			}
		} else if field.Kind() == reflect.Slice || field.Kind() == reflect.Array {
			if vType.String() == "[]uint8" || vType.String() == "[]byte" {
				field.Set(reflect.ValueOf(decoded[i].([]byte)))
			} else if vType.String() == "string" {
				fieldName := field.Type().String()
				if strings.TrimPrefix(fieldName, "*") == "common.Address" {
					field.Set(reflect.ValueOf(common.HexToAddress(decoded[i].(string))))
				} else {
					field.Set(reflect.ValueOf(decoded[i]))
				}
			} else {
				err := parseSlice(decoded[i].([]any), field.Addr().Interface())
				if err != nil {
					return fmt.Errorf("[parseStruct] error parsing slice field %s: %w", field.Type().Name(), err)
				}
			}
		} else {
			fieldName := field.Type().String()
			var val reflect.Value

			// Handle pointer fields that were excluded above
			if field.Kind() == reflect.Ptr {
				if field.Type().Elem().String() == "big.Int" {
					// *big.Int - decoded[i] should already be *big.Int
					if bi, ok := decoded[i].(*big.Int); ok {
						field.Set(reflect.ValueOf(bi))
					} else {
						return fmt.Errorf("[parseStruct] expected *big.Int, got %T", decoded[i])
					}
				} else if field.Type().Elem().String() == "common.Address" {
					// *common.Address
					addr := common.HexToAddress(decoded[i].(string))
					field.Set(reflect.ValueOf(&addr))
				} else {
					return fmt.Errorf("[parseStruct] unsupported pointer type: %s", field.Type())
				}
			} else if strings.TrimPrefix(fieldName, "*") == "common.Address" {
				val = reflect.ValueOf(common.HexToAddress(decoded[i].(string)))
				field.Set(val)
			} else {
				val = reflect.ValueOf(decoded[i])
				// Try to convert if types don't match
				if val.Type() != field.Type() {
					if val.CanConvert(field.Type()) {
						val = val.Convert(field.Type())
					} else {
						return fmt.Errorf("[parseStruct] cannot convert %T to %s", decoded[i], field.Type())
					}
				}
				field.Set(val)
			}
		}
	}

	return nil
}

func parseSlice(decoded []any, sliceVal any) error {
	rv := reflect.ValueOf(sliceVal)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("[parseSlice] v must be a pointer")
	}

	rve := rv.Elem()
	if rve.Kind() != reflect.Slice {
		return fmt.Errorf("[parseSlice] v must be a slice pointer")
	}

	arrElem := rve.Type().Elem()
	for i := range decoded {
		if arrElem.Kind() == reflect.Ptr && arrElem.Elem().String() != "big.Int" && arrElem.Elem().String() != "common.Address" {
			// Create a new pointer element (e.g., *big.Int)
			newPtr := reflect.New(arrElem.Elem())
			err := parsePointer(decoded[i].([]any), newPtr)
			if err != nil {
				return fmt.Errorf("[parseSlice] error parsing pointer field %s: %w", rve.Type().Name(), err)
			}
			rve.Set(reflect.Append(rve, newPtr))
		} else if arrElem.Kind() == reflect.Struct {
			newStruct := reflect.New(arrElem)
			err := parseStruct(decoded[i].([]any), newStruct.Interface())
			if err != nil {
				return fmt.Errorf("[parseSlice] error parsing struct field %s: %w", rve.Type().Name(), err)
			}
			rve.Set(reflect.Append(rve, newStruct.Elem()))
		} else if arrElem.Kind() == reflect.Slice || arrElem.Kind() == reflect.Array {
			err := parseSlice(decoded[i].([]any), rve.Addr().Interface())
			if err != nil {
				return fmt.Errorf("[parseSlice] error parsing slice field %s: %w", rve.Type().Name(), err)
			}
		} else {
			newElem := reflect.ValueOf(decoded[i])
			rve.Set(reflect.Append(rve, newElem))
		}
	}

	return nil
}

func parsePointer(decoded []any, pointerVal reflect.Value) error {
	if pointerVal.Kind() != reflect.Ptr {
		return fmt.Errorf("[parsePointer] v must be a pointer")
	}

	elemType := pointerVal.Type().Elem()

	if pointerVal.IsNil() {
		pointerVal.Set(reflect.New(elemType))
	}

	switch elemType.Kind() {
	case reflect.Struct:
		err := parseStruct(decoded, pointerVal.Interface())
		if err != nil {
			return fmt.Errorf("[parsePointer] error parsing struct field %s: %w", elemType.Name(), err)
		}
	case reflect.Slice, reflect.Array:
		err := parseSlice(decoded, pointerVal.Addr().Interface())
		if err != nil {
			return fmt.Errorf("[parsePointer] error parsing slice field %s: %w", elemType.Name(), err)
		}
	default:
		pointerVal.Elem().Set(reflect.ValueOf(decoded))
	}

	return nil
}
