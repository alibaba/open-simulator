package chart

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	simontype "github.com/alibaba/open-simulator/pkg/type"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/releaseutil"
)

// ProcessChart parses chart to /tmp/charts
func ProcessChart(name string, chartPath string) ([]string, error) {
	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("Func: ProcessChart | failed to load chart(%s): %v ", name, err)
	}
	chartRequested.Metadata.Name = name

	if err := checkIfInstallable(chartRequested); err != nil {
		return nil, err
	}

	// TODO
	var vals map[string]interface{}
	if err := chartutil.ProcessDependencies(chartRequested, vals); err != nil {
		return nil, fmt.Errorf("Func: ProcessChart | failed to process dependencies: %v ", err)
	}

	valuesToRender, err := ToRenderValues(chartRequested, vals)
	if err != nil {
		return nil, err
	}

	return renderResources(chartRequested, valuesToRender, true)
}

// checkIfInstallable validates if a chart can be installed
// Application chart type is only installable
func checkIfInstallable(ch *chart.Chart) error {
	switch ch.Metadata.Type {
	case "", "application":
		return nil
	}
	return fmt.Errorf("%s charts are not installable", ch.Metadata.Type)
}

// ToRenderValues composes the struct from the data coming from the Releases, Charts and Values files
func ToRenderValues(chrt *chart.Chart, chrtVals map[string]interface{}) (chartutil.Values, error) {

	top := map[string]interface{}{
		"Chart": chrt.Metadata,
		"Release": map[string]interface{}{
			"Name":      chrt.Name(),
			"Namespace": "default",
			"Revision":  1,
			"Service":   "Helm",
		},
	}

	vals, err := chartutil.CoalesceValues(chrt, chrtVals)
	if err != nil {
		return top, fmt.Errorf("Func: ToRenderValues | failed to coalesce values:%v ", err)
	}

	if err := chartutil.ValidateAgainstSchema(chrt, vals); err != nil {
		errFmt := "Func: ValidateAgainstSchema | values don't meet the specifications of the schema(s) in the following chart(s):\n%s"
		return top, fmt.Errorf(errFmt, err.Error())
	}

	top["Values"] = vals
	return top, nil
}

func renderResources(ch *chart.Chart, values chartutil.Values, subNotes bool) ([]string, error) {
	files, err := engine.Render(ch, values)
	if err != nil {
		return nil, fmt.Errorf("Func: renderResources | failed to render values into chart: %v ", err)
	}

	// NOTES.txt gets rendered like all the other files, but because it's not a hook nor a resource,
	// pull it out of here into a separate file so that we can actually use the output of the rendered
	// text file. We have to spin through this map because the file contains path information, so we
	// look for terminating NOTES.txt. We also remove it from the files so that we don't have to skip
	// it in the sortHooks.
	var notesBuffer bytes.Buffer
	for k, v := range files {
		if strings.HasSuffix(k, simontype.NotesFileSuffix) {
			if subNotes || (k == path.Join(ch.Name(), "templates", simontype.NotesFileSuffix)) {
				// If buffer contains data, add newline before adding more
				if notesBuffer.Len() > 0 {
					notesBuffer.WriteString("\n")
				}
				notesBuffer.WriteString(v)
			}
			delete(files, k)
		}
	}

	// Sort hooks, manifests, and partials. Only hooks and manifests are returned,
	// as partials are not used after renderer.Render. Empty manifests are also
	// removed here.
	var yamlStr []string
	_, manifests, err := releaseutil.SortManifests(files, []string{}, releaseutil.InstallOrder)
	if err != nil {
		return nil, fmt.Errorf("Func: renderResources | failed to sort manifests: %v ", err)
	}
	for _, item := range manifests {
		yamlStr = append(yamlStr, item.Content)
	}

	return yamlStr, nil
}
