// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/oasdiff/yaml"
)

var (
	srcFile = flag.String("swagger-file", "./api.swagger.json", "Swagger file path")
	dstFile = flag.String("openapiv3-file", "./api.yaml", "destination OpenAPI v3 file path")
)

func main() {
	input, err := os.ReadFile(*srcFile)
	if err != nil {
		panic(err)
	}

	var doc2 openapi2.T

	if err = json.Unmarshal(input, &doc2); err != nil {
		panic(err)
	}

	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		panic(err)
	}

	yml, err := doc3.MarshalYAML()
	if err != nil {
		panic(err)
	}

	output, err := yaml.Marshal(yml)
	if err != nil {
		panic(err)
	}

	if err = os.WriteFile(*dstFile, output, 0644); err != nil {
		panic(err)
	}
}
