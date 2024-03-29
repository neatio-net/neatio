package bind

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/neatio-net/neatio/chain/accounts/abi"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/crypto"
)

func makeTopics(query ...[]interface{}) ([][]common.Hash, error) {
	topics := make([][]common.Hash, len(query))
	for i, filter := range query {
		for _, rule := range filter {
			var topic common.Hash

			switch rule := rule.(type) {
			case common.Hash:
				copy(topic[:], rule[:])
			case common.Address:
				copy(topic[common.HashLength-common.AddressLength:], rule[:])
			case *big.Int:
				blob := rule.Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case bool:
				if rule {
					topic[common.HashLength-1] = 1
				}
			case int8:
				blob := big.NewInt(int64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case int16:
				blob := big.NewInt(int64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case int32:
				blob := big.NewInt(int64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case int64:
				blob := big.NewInt(rule).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint8:
				blob := new(big.Int).SetUint64(uint64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint16:
				blob := new(big.Int).SetUint64(uint64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint32:
				blob := new(big.Int).SetUint64(uint64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint64:
				blob := new(big.Int).SetUint64(rule).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case string:
				hash := crypto.Keccak256Hash([]byte(rule))
				copy(topic[:], hash[:])
			case []byte:
				hash := crypto.Keccak256Hash(rule)
				copy(topic[:], hash[:])

			default:

				val := reflect.ValueOf(rule)
				switch {

				case val.Kind() == reflect.Array && reflect.TypeOf(rule).Elem().Kind() == reflect.Uint8:
					reflect.Copy(reflect.ValueOf(topic[:val.Len()]), val)
				default:
					return nil, fmt.Errorf("unsupported indexed type: %T", rule)
				}
			}
			topics[i] = append(topics[i], topic)
		}
	}
	return topics, nil
}

var (
	reflectHash    = reflect.TypeOf(common.Hash{})
	reflectAddress = reflect.TypeOf(common.Address{})
	reflectBigInt  = reflect.TypeOf(new(big.Int))
)

func parseTopics(out interface{}, fields abi.Arguments, topics []common.Hash) error {

	if len(fields) != len(topics) {
		return errors.New("topic/field count mismatch")
	}

	for _, arg := range fields {
		if !arg.Indexed {
			return errors.New("non-indexed field in topic reconstruction")
		}
		field := reflect.ValueOf(out).Elem().FieldByName(capitalise(arg.Name))

		switch field.Kind() {
		case reflect.Bool:
			if topics[0][common.HashLength-1] == 1 {
				field.Set(reflect.ValueOf(true))
			}
		case reflect.Int8:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(int8(num.Int64())))

		case reflect.Int16:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(int16(num.Int64())))

		case reflect.Int32:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(int32(num.Int64())))

		case reflect.Int64:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(num.Int64()))

		case reflect.Uint8:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(uint8(num.Uint64())))

		case reflect.Uint16:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(uint16(num.Uint64())))

		case reflect.Uint32:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(uint32(num.Uint64())))

		case reflect.Uint64:
			num := new(big.Int).SetBytes(topics[0][:])
			field.Set(reflect.ValueOf(num.Uint64()))

		default:

			switch field.Type() {
			case reflectHash:
				field.Set(reflect.ValueOf(topics[0]))

			case reflectAddress:
				var addr common.Address
				copy(addr[:], topics[0][common.HashLength-common.AddressLength:])
				field.Set(reflect.ValueOf(addr))

			case reflectBigInt:
				num := new(big.Int).SetBytes(topics[0][:])
				if arg.Type.T == abi.IntTy {
					if num.Cmp(abi.MaxInt256) > 0 {
						num.Add(abi.MaxUint256, big.NewInt(0).Neg(num))
						num.Add(num, big.NewInt(1))
						num.Neg(num)
					}
				}
				field.Set(reflect.ValueOf(num))

			default:

				switch {

				case arg.Type.T == abi.FixedBytesTy:
					reflect.Copy(field, reflect.ValueOf(topics[0][:arg.Type.Size]))
				default:
					return fmt.Errorf("unsupported indexed type: %v", arg.Type)
				}
			}
		}
		topics = topics[1:]
	}
	return nil
}

func parseTopicsIntoMap(out map[string]interface{}, fields abi.Arguments, topics []common.Hash) error {

	if len(fields) != len(topics) {
		return errors.New("topic/field count mismatch")
	}

	for _, arg := range fields {
		if !arg.Indexed {
			return errors.New("non-indexed field in topic reconstruction")
		}

		switch arg.Type.T {
		case abi.BoolTy:
			out[arg.Name] = topics[0][common.HashLength-1] == 1
		case abi.IntTy, abi.UintTy:
			out[arg.Name] = abi.ReadInteger(arg.Type.T, arg.Type.Kind, topics[0].Bytes())
		case abi.AddressTy:
			var addr common.Address
			copy(addr[:], topics[0][common.HashLength-common.AddressLength:])
			out[arg.Name] = addr
		case abi.HashTy:
			out[arg.Name] = topics[0]
		case abi.FixedBytesTy:
			array, err := abi.ReadFixedBytes(arg.Type, topics[0].Bytes())
			if err != nil {
				return err
			}
			out[arg.Name] = array
		case abi.StringTy, abi.BytesTy, abi.SliceTy, abi.ArrayTy:

			out[arg.Name] = topics[0]
		case abi.FunctionTy:
			if garbage := binary.BigEndian.Uint64(topics[0][0:8]); garbage != 0 {
				return fmt.Errorf("bind: got improperly encoded function type, got %v", topics[0].Bytes())
			}
			var tmp [24]byte
			copy(tmp[:], topics[0][8:32])
			out[arg.Name] = tmp
		default:
			return fmt.Errorf("unsupported indexed type: %v", arg.Type)
		}

		topics = topics[1:]
	}

	return nil
}
