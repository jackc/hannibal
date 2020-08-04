package server

import (
	"errors"
	"fmt"
)

var allowedInArgs = []string{
	"query_args",
}

var allowedOutArgs = []string{
	"status",
	"resp_body",
}

type PGFuncHandler struct {
	Name    string
	InArgs  []string
	OutArgs []string
}

func NewPGFuncHandler(name string, proargmodes []string, proargnames []string) (*PGFuncHandler, error) {
	if name == "" {
		return nil, errors.New("name cannot be empty")
	}

	if len(proargmodes) == 0 {
		return nil, errors.New("proargmodes cannot be empty")
	}

	if len(proargnames) == 0 {
		return nil, errors.New("proargnames cannot be empty")
	}

	if len(proargmodes) != len(proargnames) {
		return nil, errors.New("proargmodes and proargnames are not the same length")
	}

	inArgMap := make(map[string]struct{})
	outArgMap := make(map[string]struct{})

	for i, m := range proargmodes {
		switch m {
		case "i":
			inArgMap[proargnames[i]] = struct{}{}
		case "o":
			outArgMap[proargnames[i]] = struct{}{}
		default:
			return nil, fmt.Errorf("unknown proargmode: %s", m)
		}
	}

	if _, hasStatus := outArgMap["status"]; !hasStatus {
		if _, hasRespBody := outArgMap["resp_body"]; !hasRespBody {
			return nil, errors.New("missing status and resp_body args")
		}
	}

	inArgs := make([]string, 0, len(inArgMap))
	// Allowed input arguments in order.
	for _, a := range allowedInArgs {
		if _, ok := inArgMap[a]; ok {
			inArgs = append(inArgs, a)
			delete(inArgMap, a)
		}
	}

	outArgs := make([]string, 0, len(outArgMap))
	// Allowed input arguments in order.
	for _, a := range allowedOutArgs {
		if _, ok := outArgMap[a]; ok {
			outArgs = append(outArgs, a)
			delete(outArgMap, a)
		}
	}

	// inArgMap should be empty
	if len(inArgMap) > 0 {
		for k, _ := range inArgMap {
			return nil, fmt.Errorf("unknown arg: %s", k)
		}
	}

	// outArgMap should be empty
	if len(outArgMap) > 0 {
		for k, _ := range outArgMap {
			return nil, fmt.Errorf("unknown arg: %s", k)
		}
	}

	h := &PGFuncHandler{
		Name:    name,
		InArgs:  inArgs,
		OutArgs: outArgs,
	}

	return h, nil
}
