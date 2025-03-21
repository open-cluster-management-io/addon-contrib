package applier

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/openshift/library-go/pkg/assets"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// GetConfigValuesFunc is function type that returns the configuration values for the given profile
type GetConfigValuesFunc func(profile string) (interface{}, error)

// Renderer is the interface for the template renderer
type Renderer interface {
	Render(component, profile string, getConfigValuesFunc GetConfigValuesFunc) ([]*unstructured.Unstructured, error)
	RenderWithFilter(component, profile, filterOut string, getConfigValuesFunc GetConfigValuesFunc) (
		[]*unstructured.Unstructured, error)
}

// renderer is an implementation of the Renderer interface for multicluster-global-hub scenario
type renderer struct {
	manifestFS embed.FS
	decoder    runtime.Decoder
}

// NewRenderer create a HoHRenderer with given filesystem
func NewRenderer(manifestFS embed.FS) Renderer {
	return &renderer{
		manifestFS: manifestFS,
		decoder:    yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme),
	}
}

func (r *renderer) Render(component, profile string, getConfigValuesFunc GetConfigValuesFunc) (
	[]*unstructured.Unstructured, error,
) {
	return r.RenderWithFilter(component, profile, "", getConfigValuesFunc)
}

func (r *renderer) RenderWithFilter(component, profile, filter string, getConfigValuesFunc GetConfigValuesFunc) (
	[]*unstructured.Unstructured, error,
) {
	var unstructuredObjs []*unstructured.Unstructured

	configValues, err := getConfigValuesFunc(profile)
	if err != nil {
		return unstructuredObjs, err
	}

	templateFiles, err := getTemplateFiles(r.manifestFS, component, filter)
	if err != nil {
		return unstructuredObjs, err
	}
	if len(templateFiles) == 0 {
		return unstructuredObjs, fmt.Errorf("no template files found")
	}

	for _, template := range templateFiles {
		templateContent, err := r.manifestFS.ReadFile(template)
		if err != nil {
			return unstructuredObjs, err
		}

		if len(templateContent) == 0 {
			continue
		}

		raw := assets.MustCreateAssetFromTemplate(template, templateContent, configValues).Data
		yamlReader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
		for {
			b, err := yamlReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return unstructuredObjs, err
			}
			if len(b) != 0 {
				object, _, err := r.decoder.Decode(b, nil, nil)
				if err != nil && runtime.IsMissingKind(err) {
					continue
				} else if err != nil {
					return unstructuredObjs, err
				}

				unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
				if err != nil {
					return unstructuredObjs, err
				}

				unstructuredObjs = append(unstructuredObjs, &unstructured.Unstructured{
					Object: unstructuredObj,
				})
			}
		}
	}

	return unstructuredObjs, nil
}

func getTemplateFiles(manifestFS embed.FS, dir, filter string) ([]string, error) {
	files, err := getFiles(manifestFS)
	if err != nil {
		return nil, err
	}
	if dir == "." || len(dir) == 0 {
		return files, nil
	}

	var templateFiles []string
	for _, file := range files {
		if strings.HasPrefix(file, dir) && strings.Contains(file, filter) {
			templateFiles = append(templateFiles, file)
		}
	}

	return templateFiles, nil
}

func getFiles(manifestFS embed.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(manifestFS, ".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, file)
		return nil
	})
	return files, err
}
