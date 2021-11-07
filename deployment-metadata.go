package main

import (
	"fmt"
)

type DeploymentMetadata struct {
	Type     string   `json:"type"`
	Service  string   `json:"service"`
	Machines []string `json:"machines"`
}

func (m *DeploymentMetadata) Validate() error {
	isValidType := map[string]bool{
		"OKCONTENT": true,
		"WEBSRV":    true,
		"RPCSRV":    true,
		"DBPROX":    true,
		"OKAPI":     true,
		"GRPC":      true,
		"CONF":      true,
	}
	if !isValidType[m.Type] {
		return fmt.Errorf("invalid deployment type \"%s\"", m.Type)
	}

	requiresService := map[string]bool{
		"RPCSRV": true,
		"DBPROX": true,
		"GRPC":   true,
	}
	if requiresService[m.Type] && len(m.Service) == 0 {
		return fmt.Errorf(
			"deployment type \"%s\" requires non-empty \"service\" field",
			m.Type,
		)
	}

	if len(m.Machines) == 0 {
		return fmt.Errorf("must specify \"machines\" list for a %s deployment", m.Type)
	}

	return nil
}
