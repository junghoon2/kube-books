/*
Copyright 2020 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
)

func TestCephLabelsMerge(t *testing.T) {
	// No Labels defined
	testLabels := LabelsSpec{}
	a := GetOSDLabels(testLabels)
	assert.Nil(t, a)

	// Only a specific component labels without "all"
	testLabels = LabelsSpec{
		"mgr":       {"mgrkey": "mgrval"},
		"mon":       {"monkey": "monval"},
		"osd":       {"osdkey": "osdval"},
		"rgw":       {"rgwkey": "rgwval"},
		"rbdmirror": {"rbdmirrorkey": "rbdmirrorval"},
	}
	a = GetMgrLabels(testLabels)
	assert.Equal(t, "mgrval", a["mgrkey"])
	assert.Equal(t, 1, len(a))
	a = GetMonLabels(testLabels)
	assert.Equal(t, "monval", a["monkey"])
	assert.Equal(t, 1, len(a))
	a = GetOSDLabels(testLabels)
	assert.Equal(t, "osdval", a["osdkey"])
	assert.Equal(t, 1, len(a))

	// No Labels matching the component
	testLabels = LabelsSpec{
		"mgr": {"mgrkey": "mgrval"},
	}
	a = GetMonLabels(testLabels)
	assert.Nil(t, a)

	// Merge with "all"
	testLabels = LabelsSpec{
		"all": {"allkey1": "allval1", "allkey2": "allval2"},
		"mgr": {"mgrkey": "mgrval"},
	}
	a = GetMonLabels(testLabels)
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 2, len(a))
	a = GetMgrLabels(testLabels)
	assert.Equal(t, "mgrval", a["mgrkey"])
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 3, len(a))
}

func TestLabelsSpec(t *testing.T) {
	specYaml := []byte(`
mgr:
  foo: bar
  hello: world
mon:
`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.YAMLToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed Labels spec object
	var Labels LabelsSpec
	err = json.Unmarshal(rawJSON, &Labels)
	assert.Nil(t, err)

	// the unmarshalled Labels spec should equal the expected spec below
	expected := LabelsSpec{
		"mgr": map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
		"mon": nil,
	}
	assert.Equal(t, expected, Labels)
}
