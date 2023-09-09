/*
Copyright 2023 Stefan Prodan

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

package runtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ResourceReader fetches resources from the cluster and extract field values.
type ResourceReader struct {
	rm *ssa.ResourceManager
}

// NewResourceReader creates a resource reader for the given cluster.
func NewResourceReader(resManager *ssa.ResourceManager) *ResourceReader {
	return &ResourceReader{
		rm: resManager,
	}
}

// Read fetches the resources from the cluster and runs the CUE expressions
// to select the desired values.
func (r *ResourceReader) Read(ctx context.Context, refs []apiv1.RuntimeResourceRef) (map[string]string, error) {
	result := make(map[string]string)

	for _, ref := range refs {
		obj, err := r.getObject(ctx, ref)
		if err != nil {
			if ref.Optional && apierrors.IsNotFound(err) {
				continue
			}

			return result, fmt.Errorf("query error for %s: %w", ref.Name, err)
		}

		ct := cuecontext.New()
		m, err := r.getValues(ct, obj, ref.Expressions)
		if err != nil {
			return result, fmt.Errorf("can't extract values from %s: %w", ssa.FmtUnstructured(obj), err)
		}

		maps.Copy(result, m)
	}

	return result, nil
}

func (r *ResourceReader) getObject(ctx context.Context, in apiv1.RuntimeResourceRef) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(in.APIVersion)
	obj.SetKind(in.Kind)
	obj.SetName(in.Name)
	obj.SetNamespace(in.Namespace)

	objKey := client.ObjectKeyFromObject(obj)
	err := r.rm.Client().Get(ctx, objKey, obj)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (r *ResourceReader) getValues(ctx *cue.Context, obj *unstructured.Unstructured, selectors map[string]string) (map[string]string, error) {
	v := ctx.Encode(obj)
	if v.Err() != nil {
		return selectors, fmt.Errorf("encoding error: %w", v.Err())
	}

	result := make(map[string]string)
	for key, exp := range selectors {
		shell := ctx.CompileString(fmt.Sprintf(`
		obj: %v
		out: %s
	`, v, exp))
		if shell.Err() != nil {
			return selectors, fmt.Errorf("%s compile error: %w", key, shell.Err())
		}

		res := shell.LookupPath(cue.ParsePath("out"))
		if res.Err() != nil {
			return selectors, fmt.Errorf("%s lookup path error: %w", key, res.Err())
		}

		switch res.IncompleteKind() {
		case cue.StringKind:
			s, _ := res.String()
			result[key] = s
		case cue.StructKind:
			return result, fmt.Errorf("unsupported type retuned by '%s'", exp)
		case cue.ListKind:
			return result, fmt.Errorf("unsupported type retuned by '%s'", exp)
		default:
			result[key] = fmt.Sprintf("%v", res)
		}

		// Decode Secret data from base64
		if obj.GetAPIVersion() == "v1" && obj.GetKind() == "Secret" {
			if data, err := base64.StdEncoding.DecodeString(result[key]); err == nil {
				result[key] = string(data)
			}
		}
	}

	return result, nil
}
