package chart

import (
	"bytes"
	"fmt"
	simontype "github.com/alibaba/open-simulator/pkg/type"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/releaseutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ProcessChart parses chart to /tmp/charts
func ProcessChart(chartPath string) error {
	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	if err := checkIfInstallable(chartRequested); err != nil {
		return err
	}

	// TODO
	var vals map[string]interface{}
	if err := chartutil.ProcessDependencies(chartRequested, vals); err != nil {
		return err
	}

	valuesToRender, err := ToRenderValues(chartRequested, vals)
	if err != nil {
		return err
	}

	if err = renderResources(chartRequested, valuesToRender, true, simontype.DirectoryForChart); err != nil {
		return err
	}

	return nil
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
			"Name":      "test",
			"Namespace": "default",
			"Revision":  1,
			"Service":   "Helm",
		},
	}

	vals, err := chartutil.CoalesceValues(chrt, chrtVals)
	if err != nil {
		return top, err
	}

	if err := chartutil.ValidateAgainstSchema(chrt, vals); err != nil {
		errFmt := "values don't meet the specifications of the schema(s) in the following chart(s):\n%s"
		return top, fmt.Errorf(errFmt, err.Error())
	}

	top["Values"] = vals
	return top, nil
}

func renderResources(ch *chart.Chart, values chartutil.Values, subNotes bool, outputDir string) error {
	files, err := engine.Render(ch, values)
	if err != nil {
		return err
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
	_, manifests, err := releaseutil.SortManifests(files, []string{}, releaseutil.InstallOrder)
	if err != nil {
		return err
	}

	// fileWritten stores a written file so that we can recognize whether the same named file has been written .
	// if it exists, we rename it and create it by new name. It ensures that one file only contains one
	// kubernetes resource object.
	fileWritten := make(map[string]bool)
	for _, m := range manifests {
		newName := m.Name
		for i := 1; i < 100; i++ {
			if _, exist := fileWritten[newName]; exist {
				newName = strings.Replace(m.Name, ".y", fmt.Sprintf("-%d.y", i), 1)
				continue
			}
			fileWritten[newName] = true
			break
		}
		err = writeToFile(outputDir, newName, m.Content)
		if err != nil {
			return err
		}
	}

	return nil
}

// writeToFile write the <data> to <output-dir>/<name>. if the file exists, we cover it.
func writeToFile(outputDir string, name string, data string) error {
	outfileName := strings.Join([]string{outputDir, name}, string(filepath.Separator))

	err := ensureDirectoryForFile(outfileName)
	if err != nil {
		return err
	}

	f, err := createFile(outfileName)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("%s\n", data))
	if err != nil {
		return err
	}

	return nil
}

func createFile(filename string) (*os.File, error) {
	return os.Create(filename)
}

// check if the directory exists to create file. creates if don't exists
func ensureDirectoryForFile(file string) error {
	baseDir := path.Dir(file)
	_, err := os.Stat(baseDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return os.MkdirAll(baseDir, simontype.DefaultDirectoryPermission)
}
