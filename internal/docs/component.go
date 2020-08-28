package docs

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"github.com/Jeffail/benthos/v3/lib/util/config"
	"github.com/Jeffail/gabs/v2"
	"gopkg.in/yaml.v3"
)

// AnnotatedExample is an isolated example for a component.
type AnnotatedExample struct {
	// A title for the example.
	Title string

	// Summary of the example.
	Summary string

	// A config snippet to show.
	Config string
}

// ComponentSpec describes a Benthos component.
type ComponentSpec struct {
	// Name of the component
	Name string

	// Type of the component (input, output, etc)
	Type string

	// Summary of the component (in markdown, must be short).
	Summary string

	// Description of the component (in markdown).
	Description string

	// Footnotes of the component (in markdown).
	Footnotes string

	// Examples demonstrating use cases for the component.
	Examples []AnnotatedExample

	// Whether the component is beta.
	Beta bool

	// Whether the component is now deprecated.
	Deprecated bool

	Fields FieldSpecs
}

type fieldContext struct {
	Name          string
	Type          string
	Description   string
	Default       string
	Advanced      bool
	Deprecated    bool
	Interpolation FieldInterpolation
	Examples      []string
	Options       []string
}

type componentContext struct {
	Name           string
	Type           string
	Summary        string
	Description    string
	Examples       []AnnotatedExample
	Fields         []fieldContext
	Footnotes      string
	CommonConfig   string
	AdvancedConfig string
	Beta           bool
	Deprecated     bool
}

func (ctx fieldContext) InterpolationBatchWide() FieldInterpolation {
	return FieldInterpolationBatchWide
}

func (ctx fieldContext) InterpolationIndividual() FieldInterpolation {
	return FieldInterpolationIndividual
}

var componentTemplate = `{{define "field_docs" -}}
## Fields

{{range $i, $field := .Fields -}}
### ` + "`{{$field.Name}}`" + `

{{$field.Description}}
{{if eq $field.Interpolation .InterpolationBatchWide -}}
This field supports [interpolation functions](/docs/configuration/interpolation#bloblang-queries).
{{end -}}
{{if eq $field.Interpolation .InterpolationIndividual -}}
This field supports [interpolation functions](/docs/configuration/interpolation#bloblang-queries).
{{end}}

Type: ` + "`{{$field.Type}}`" + `  
{{if gt (len $field.Default) 0}}Default: ` + "`{{$field.Default}}`" + `  
{{end -}}
{{if gt (len $field.Options) 0}}Options: {{range $j, $option := $field.Options -}}
{{if ne $j 0}}, {{end}}` + "`" + `{{$option}}` + "`" + `{{end}}.
{{end}}
{{if gt (len $field.Examples) 0 -}}
` + "```yaml" + `
# Examples

{{range $j, $example := $field.Examples -}}
{{if ne $j 0}}
{{end}}{{$example}}{{end -}}
` + "```" + `

{{end -}}
{{end -}}
{{end -}}

---
title: {{.Name}}
type: {{.Type}}
{{if .Beta -}}
beta: true
{{end -}}
{{if .Deprecated -}}
deprecated: true
{{end -}}
---

<!--
     THIS FILE IS AUTOGENERATED!

     To make changes please edit the contents of:
     lib/{{.Type}}/{{.Name}}.go
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

{{if .Beta -}}
BETA: This component is experimental and therefore subject to change outside of
major version releases.
{{end -}}
{{if .Deprecated -}}
DEPRECATED: This component is deprecated and will be removed in the next major
version release. Please consider moving onto [alternative components](#alternatives).
{{end -}}

{{if gt (len .Summary) 0 -}}
{{.Summary}}
{{end}}
{{if eq .CommonConfig .AdvancedConfig -}}
` + "```yaml" + `
# Config fields, showing default values
{{.CommonConfig -}}
` + "```" + `
{{else}}
<Tabs defaultValue="common" values={{"{"}}[
  { label: 'Common', value: 'common', },
  { label: 'Advanced', value: 'advanced', },
]{{"}"}}>

<TabItem value="common">

` + "```yaml" + `
# Common config fields, showing default values
{{.CommonConfig -}}
` + "```" + `

</TabItem>
<TabItem value="advanced">

` + "```yaml" + `
# All config fields, showing default values
{{.AdvancedConfig -}}
` + "```" + `

</TabItem>
</Tabs>
{{end -}}
{{if gt (len .Description) 0}}
{{.Description}}
{{end}}
{{if and (le (len .Fields) 4) (gt (len .Fields) 0) -}}
{{template "field_docs" . -}}
{{end -}}

{{if gt (len .Examples) 0 -}}
## Examples

<Tabs defaultValue="{{ (index .Examples 0).Title }}" values={{"{"}}[
{{range $i, $example := .Examples -}}
  { label: '{{$example.Title}}', value: '{{$example.Title}}', },
{{end -}}
]{{"}"}}>

{{range $i, $example := .Examples -}}
<TabItem value="{{$example.Title}}">

{{if gt (len $example.Summary) 0 -}}
{{$example.Summary}}
{{end}}
{{if gt (len $example.Config) 0 -}}
` + "```yaml" + `{{$example.Config}}` + "```" + `
{{end}}
</TabItem>
{{end -}}
</Tabs>

{{end -}}

{{if gt (len .Fields) 4 -}}
{{template "field_docs" . -}}
{{end -}}

{{if gt (len .Footnotes) 0 -}}
{{.Footnotes}}
{{end}}
`

func (c *ComponentSpec) createConfigs(root string, fullConfigExample interface{}) (
	advancedConfigBytes, commonConfigBytes []byte,
) {
	var err error
	if len(c.Fields) > 0 {
		advancedConfig, err := c.Fields.ConfigAdvanced(fullConfigExample)
		if err == nil {
			tmp := map[string]interface{}{
				c.Name: advancedConfig,
			}
			if len(root) > 0 {
				tmp = map[string]interface{}{
					root: tmp,
				}
			}
			advancedConfigBytes, err = config.MarshalYAML(tmp)
		}
		var commonConfig interface{}
		if err == nil {
			commonConfig, err = c.Fields.ConfigCommon(advancedConfig)
		}
		if err == nil {
			tmp := map[string]interface{}{
				c.Name: commonConfig,
			}
			if len(root) > 0 {
				tmp = map[string]interface{}{
					root: tmp,
				}
			}
			commonConfigBytes, err = config.MarshalYAML(tmp)
		}
	}
	if err != nil {
		panic(err)
	}
	if len(c.Fields) == 0 {
		tmp := map[string]interface{}{
			c.Name: fullConfigExample,
		}
		if len(root) > 0 {
			tmp = map[string]interface{}{
				root: tmp,
			}
		}
		if advancedConfigBytes, err = config.MarshalYAML(tmp); err != nil {
			panic(err)
		}
		commonConfigBytes = advancedConfigBytes
	}
	return
}

// AsMarkdown renders the spec of a component, along with a full configuration
// example, into a markdown document.
func (c *ComponentSpec) AsMarkdown(nest bool, fullConfigExample interface{}) ([]byte, error) {
	ctx := componentContext{
		Name:        c.Name,
		Type:        c.Type,
		Summary:     c.Summary,
		Description: c.Description,
		Examples:    c.Examples,
		Footnotes:   c.Footnotes,
		Beta:        c.Beta,
		Deprecated:  c.Deprecated,
	}

	if tmpBytes, err := yaml.Marshal(fullConfigExample); err == nil {
		fullConfigExample = map[string]interface{}{}
		if err = yaml.Unmarshal(tmpBytes, &fullConfigExample); err != nil {
			panic(err)
		}
	} else {
		panic(err)
	}

	root := ""
	if nest {
		root = c.Type
	}

	advancedConfigBytes, commonConfigBytes := c.createConfigs(root, fullConfigExample)
	ctx.CommonConfig = string(commonConfigBytes)
	ctx.AdvancedConfig = string(advancedConfigBytes)

	gConf := gabs.Wrap(fullConfigExample)

	if len(c.Description) > 0 && c.Description[0] == '\n' {
		ctx.Description = c.Description[1:]
	}
	if len(c.Footnotes) > 0 && c.Footnotes[0] == '\n' {
		ctx.Footnotes = c.Footnotes[1:]
	}

	flattenedFields := FieldSpecs{}
	var walkFields func(path string, gObj *gabs.Container, f FieldSpecs) ([]string, []string)
	walkFields = func(path string, gObj *gabs.Container, f FieldSpecs) ([]string, []string) {
		var missingFields []string
		expectedFields := map[string]struct{}{}
		for k := range gObj.ChildrenMap() {
			expectedFields[k] = struct{}{}
		}
		seenFields := map[string]struct{}{}
		var duplicateFields []string
		for _, v := range f {
			if _, seen := seenFields[v.Name]; seen {
				duplicateFields = append(duplicateFields, v.Name)
			}
			seenFields[v.Name] = struct{}{}
			newV := v
			delete(expectedFields, v.Name)
			if len(path) > 0 {
				newV.Name = path + newV.Name
			}
			flattenedFields = append(flattenedFields, newV)
			if len(v.Children) > 0 {
				tmpMissing, tmpDuplicate := walkFields(path+v.Name+".", gConf.S(v.Name), v.Children)
				missingFields = append(missingFields, tmpMissing...)
				duplicateFields = append(duplicateFields, tmpDuplicate...)
			}
		}
		for k := range expectedFields {
			missingFields = append(missingFields, path+k)
		}
		return missingFields, duplicateFields
	}
	if len(c.Fields) > 0 {
		if missing, duplicates := walkFields("", gConf, c.Fields); len(missing) > 0 {
			return nil, fmt.Errorf("spec missing fields: %v", missing)
		} else if len(duplicates) > 0 {
			return nil, fmt.Errorf("spec duplicate fields: %v", duplicates)
		}
	}

	for _, v := range flattenedFields {
		if v.Deprecated {
			continue
		}

		if !gConf.ExistsP(v.Name) {
			return nil, fmt.Errorf("unrecognised field '%v'", v.Name)
		}

		defaultValue := gConf.Path(v.Name)
		if defaultValue.Data() == nil {
			return nil, fmt.Errorf("field '%v' not found in config example", v.Name)
		}

		defaultValueStr := defaultValue.String()
		if len(v.Children) > 0 {
			defaultValueStr = ""
		}

		fieldType := v.Type
		if len(fieldType) == 0 {
			if len(v.Examples) > 0 {
				fieldType = reflect.TypeOf(v.Examples[0]).Kind().String()
			} else {
				fieldType = reflect.TypeOf(defaultValue.Data()).Kind().String()
			}
		}
		switch fieldType {
		case "map":
			fieldType = "object"
		case "slice":
			fieldType = "array"
		case "float64", "int", "int64":
			fieldType = "number"
		}

		var examples []string
		if len(v.Examples) > 0 {
			nameSplit := strings.Split(v.Name, ".")
			exampleName := nameSplit[len(nameSplit)-1]
			for _, example := range v.Examples {
				exampleBytes, err := config.MarshalYAML(map[string]interface{}{
					exampleName: example,
				})
				if err != nil {
					return nil, err
				}
				examples = append(examples, string(exampleBytes))
			}
		}

		fieldCtx := fieldContext{
			Name:          v.Name,
			Type:          fieldType,
			Description:   v.Description,
			Default:       defaultValueStr,
			Advanced:      v.Advanced,
			Examples:      examples,
			Options:       v.Options,
			Interpolation: v.Interpolation,
		}

		if len(fieldCtx.Description) == 0 {
			fieldCtx.Description = "Sorry! This field is missing documentation."
		}

		if fieldCtx.Description[0] == '\n' {
			fieldCtx.Description = fieldCtx.Description[1:]
		}

		ctx.Fields = append(ctx.Fields, fieldCtx)
	}

	var buf bytes.Buffer
	err := template.Must(template.New("component").Parse(componentTemplate)).Execute(&buf, ctx)

	return buf.Bytes(), err
}
