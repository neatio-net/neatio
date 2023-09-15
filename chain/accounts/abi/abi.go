package abi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/nio-net/nio/utilities/common"
)

type ABI struct {
	Constructor Method
	Methods     map[string]Method
	Events      map[string]Event
}

func JSON(reader io.Reader) (ABI, error) {
	dec := json.NewDecoder(reader)

	var abi ABI
	if err := dec.Decode(&abi); err != nil {
		return ABI{}, err
	}

	return abi, nil
}

func (abi ABI) Pack(name string, args ...interface{}) ([]byte, error) {

	if name == "" {

		arguments, err := abi.Constructor.Inputs.Pack(args...)
		if err != nil {
			return nil, err
		}
		return arguments, nil

	}
	method, exist := abi.Methods[name]
	if !exist {
		return nil, fmt.Errorf("method '%s' not found", name)
	}

	arguments, err := method.Inputs.Pack(args...)
	if err != nil {
		return nil, err
	}

	return append(method.ID(), arguments...), nil
}

func (abi ABI) Unpack(v interface{}, name string, data []byte) (err error) {

	if method, ok := abi.Methods[name]; ok {
		if len(data)%32 != 0 {
			return fmt.Errorf("abi: improperly formatted output: %s - Bytes: [%+v]", string(data), data)
		}
		return method.Outputs.Unpack(v, data)
	}
	if event, ok := abi.Events[name]; ok {
		return event.Inputs.Unpack(v, data)
	}
	return fmt.Errorf("abi: could not locate named method or event")
}

func (abi ABI) UnpackIntoMap(v map[string]interface{}, name string, data []byte) (err error) {

	if method, ok := abi.Methods[name]; ok {
		if len(data)%32 != 0 {
			return fmt.Errorf("abi: improperly formatted output")
		}
		return method.Outputs.UnpackIntoMap(v, data)
	}
	if event, ok := abi.Events[name]; ok {
		return event.Inputs.UnpackIntoMap(v, data)
	}
	return fmt.Errorf("abi: could not locate named method or event")
}

func (abi ABI) UnpackMethodInputs(v interface{}, name string, input []byte) (err error) {
	if len(input) == 0 {
		return fmt.Errorf("abi: unmarshalling empty output")
	}
	if method, ok := abi.Methods[name]; ok {
		return method.Inputs.Unpack(v, input)
	}
	return fmt.Errorf("abi: could not locate named method or event")
}

func (abi *ABI) UnmarshalJSON(data []byte) error {
	var fields []struct {
		Type            string
		Name            string
		Constant        bool
		StateMutability string
		Anonymous       bool
		Inputs          []Argument
		Outputs         []Argument
	}

	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	abi.Methods = make(map[string]Method)
	abi.Events = make(map[string]Event)
	for _, field := range fields {
		switch field.Type {
		case "constructor":
			abi.Constructor = Method{
				Inputs: field.Inputs,
			}

		case "function", "":
			name := field.Name
			_, ok := abi.Methods[name]
			for idx := 0; ok; idx++ {
				name = fmt.Sprintf("%s%d", field.Name, idx)
				_, ok = abi.Methods[name]
			}
			isConst := field.Constant || field.StateMutability == "pure" || field.StateMutability == "view"
			abi.Methods[name] = Method{
				Name:    name,
				RawName: field.Name,
				Const:   isConst,
				Inputs:  field.Inputs,
				Outputs: field.Outputs,
			}
		case "event":
			name := field.Name
			_, ok := abi.Events[name]
			for idx := 0; ok; idx++ {
				name = fmt.Sprintf("%s%d", field.Name, idx)
				_, ok = abi.Events[name]
			}
			abi.Events[name] = Event{
				Name:      name,
				RawName:   field.Name,
				Anonymous: field.Anonymous,
				Inputs:    field.Inputs,
			}
		}
	}

	return nil
}

func (abi *ABI) MethodById(sigdata []byte) (*Method, error) {
	if len(sigdata) < 4 {
		return nil, fmt.Errorf("data too short (%d bytes) for abi method lookup", len(sigdata))
	}
	for _, method := range abi.Methods {
		if bytes.Equal(method.ID(), sigdata[:4]) {
			return &method, nil
		}
	}
	return nil, fmt.Errorf("no method with id: %#x", sigdata[:4])
}

func (abi *ABI) EventByID(topic common.Hash) (*Event, error) {
	for _, event := range abi.Events {
		if bytes.Equal(event.ID().Bytes(), topic.Bytes()) {
			return &event, nil
		}
	}
	return nil, fmt.Errorf("no event with id: %#x", topic.Hex())
}
