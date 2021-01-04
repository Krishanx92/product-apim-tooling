package k8s

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	wso2v1alpha2 "github.com/wso2/k8s-api-operator/api-operator/pkg/apis/wso2/v1alpha2"
	"github.com/wso2/product-apim-tooling/import-export-cli/box"
	k8sUtils "github.com/wso2/product-apim-tooling/import-export-cli/operator/utils"
	"github.com/wso2/product-apim-tooling/import-export-cli/utils"
)

var flagApiName string
var flagSwaggerFilePath string
var flagReplicas int
var flagNamespace string
var flagApiVersion string
var flagApiMode string

const AddApiCmdLiteral = "api"
const addApiCmdShortDesc = "Handle APIs in kubernetes cluster "
const addApiLongDesc = `Add, Update and Delete APIs in kubernetes cluster. JSON and YAML formats are accepted.
available modes are as follows
* kubernetes`
const addApiExamples = utils.ProjectName + " " + K8sCmdLiteral + " add/update " + AddApiCmdLiteral +
	` -n petstore --from-file=./Swagger.json --replicas=3 --namespace=wso2`

// addApiCmd represents the api command
var addApiCmd = &cobra.Command{
	Use:     AddApiCmdLiteral,
	Short:   addApiCmdShortDesc,
	Long:    addApiLongDesc,
	Example: addApiExamples,
	Run: func(cmd *cobra.Command, args []string) {
		utils.Logln(utils.LogPrefixInfo + AddApiCmdLiteral + " called")
		handleAddApi("")
	},
}

func handleAddApi(nameSuffix string) {
	validateAddApiCommand()

	//swaggerCmNames := make([]string, len(flagSwaggerFilePaths))
	//balInterceptorsCmNames := make([]string, 0, len(flagSwaggerFilePaths))
	//var javaInterceptorsCmNames []string

	// log processing only if there are more projects
	utils.Logln(fmt.Sprintf("%sProcessing swagger  %v", utils.LogPrefixInfo, flagSwaggerFilePath))

	flagApiName = strings.ToLower(flagApiName)
	swaggerCmName := fmt.Sprintf("%v-swagger%s", flagApiName, nameSuffix)

	fi, _ := os.Stat(flagSwaggerFilePath) // error already handled and ignore error
	switch mode := fi.Mode(); {
	//check if the swagger path is a Dir
	case mode.IsDir():
		//get swagger definition
		swaggerPath := filepath.Join(flagSwaggerFilePath, filepath.FromSlash("Meta-information/swagger.yaml"))
		//creating kubernetes configmap with swagger definition
		fmt.Println("creating configmap with swagger definition")
		errConf := createConfigMapWithNamespace(swaggerCmName, swaggerPath, flagNamespace, k8sUtils.K8sCreate)
		if errConf != nil {
			utils.HandleErrorAndExit("Error creating configmap", errConf)
		}

	//check if the swagger path is a file
	case mode.IsRegular():
		//creating kubernetes configmap with swagger definition
		fmt.Println("creating configmap with swagger definition")
		errConf := createConfigMapWithNamespace(swaggerCmName, flagSwaggerFilePath, flagNamespace,
			k8sUtils.K8sCreate)
		if errConf != nil {
			utils.HandleErrorAndExit("Error creating configmap", errConf)
		}
	}

	//create API
	fmt.Println("creating API definition")
	createAPI(swaggerCmName, nameSuffix)
}

// validateAddApiCommand validates for required flags and if invalid print error and exit
func validateAddApiCommand() {
	// validate mode
	//configVars := utils.GetMainConfigFromFile(utils.MainConfigFilePath)
	//if !configVars.Config.KubernetesMode {
	//	utils.HandleErrorAndExit("set mode to kubernetes with command: apictl set --mode kubernetes",
	//		errors.New("mode should be set to kubernetes"))
	//}

	// validate --from-file flag values
	if _, err := os.Stat(flagSwaggerFilePath); err != nil {
		utils.HandleErrorAndExit("swagger file path or project not found", err)
	}
}

//create configmap with swagger definition
func createConfigMapWithNamespace(configMapName string, filePath string, namespace string, operation string) error {
	cmd := exec.Command(
		k8sUtils.Kubectl,
		operation,
		"configmap",
		configMapName,
		"--from-file",
		filePath,
		"-n", namespace,
	)
	//print kubernetes error commands
	var errBuf, outBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func createAPI(configMapName string, timestamp string) {
	//get API definition from file
	apiConfigMapData, _ := box.Get("/kubernetes_resources/api_cr.yaml")
	apiCrd := &wso2v1alpha2.API{}
	errUnmarshal := yaml.Unmarshal(apiConfigMapData, apiCrd)
	if errUnmarshal != nil {
		utils.HandleErrorAndExit("Error unmarshal api configmap into struct ", errUnmarshal)
	}
	//assigning values to API cr
	apiCrd.Name = flagApiName
	apiCrd.Namespace = flagNamespace
	apiCrd.Spec.SwaggerConfigMapName = configMapName

	k8sOperation := k8sUtils.K8sCreate
	k8sSaveConfig := true
	if timestamp != "" {
		//set update timestamp
		apiCrd.Spec.UpdateTimeStamp = timestamp
		k8sOperation = k8sUtils.K8sApply
		k8sSaveConfig = false
	}


	byteVal, errMarshal := yaml.Marshal(apiCrd)
	if errMarshal != nil {
		utils.HandleErrorAndExit("Error marshal api configmap ", errMarshal)
	}
	//write configmap to a temp file
	tmpFile, err := ioutil.TempFile(os.TempDir(), "apicr-*.yaml")
	if err != nil {
		log.Fatal("Cannot create temporary file", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err = tmpFile.Write(byteVal); err != nil {
		log.Fatal("Failed to write to temporary file", err)
	}
	// Close the file
	if err := tmpFile.Close(); err != nil {
		log.Fatal(err)
	}

	k8sArgs := []string{k8sOperation, "-f", tmpFile.Name(), "-n", flagNamespace}
	if k8sSaveConfig {
		k8sArgs = append(k8sArgs, "--save-config")
	}

	//execute kubernetes command to create or update api from file
	errAddApi := k8sUtils.ExecuteCommand(k8sUtils.Kubectl, k8sArgs...)
	if errAddApi != nil {
		fmt.Println("error configuring API")
		// delete all configs if any error
		rollbackConfigs(apiCrd)
	}
}

// rollbackConfigs deletes configs defined in the API CR given
func rollbackConfigs(apiCr *wso2v1alpha2.API) {
	var rollbackConfMaps []string // configmap names to be deleted

	// swagger configmaps
	rollbackConfMaps = append(rollbackConfMaps, apiCr.Spec.SwaggerConfigMapName)

	if len(rollbackConfMaps) == 0 {
		return
	}

	// execute kubernetes command to delete
	fmt.Println("Deleting created configs")
	k8sArgs := []string{k8sUtils.K8sDelete, "cm"}
	k8sArgs = append(k8sArgs, rollbackConfMaps...)

	delConfErr := k8sUtils.ExecuteCommand(k8sUtils.Kubectl, k8sArgs...)
	if delConfErr != nil {
		utils.HandleErrorAndExit("error deleting configmaps of the API: "+apiCr.Name, delConfErr)
	}
}

func init() {
	K8sAddCmd.AddCommand(addApiCmd)
	//addApiCmd.Flags().StringVarP(&flagApiEndPoint, "apiEndPoint", "a", "", "")
	addApiCmd.Flags().StringVarP(&flagApiName, "name", "n", "", "Name of the API")
	addApiCmd.Flags().StringVarP(&flagSwaggerFilePath, "from-file", "f", "",
		"Path to swagger file")
	//addApiCmd.Flags().IntVar(&flagReplicas, "replicas", 1, "replica set")
	addApiCmd.Flags().StringVar(&flagNamespace, "namespace", "", "namespace of API")
	//addApiCmd.Flags().BoolVarP(&flagOverride, "override", "", false,
	//	"Property to override the existing docker image with the given name and version")
	//addApiCmd.Flags().StringVarP(&flagApiVersion, "version", "v", "",
	//	"Property to override the API version")
	//addApiCmd.Flags().StringVarP(&flagApiMode, "mode", "m", "",
	//	fmt.Sprintf("Property to override the deploying mode. Available modes: %v, %v",
	//		utils.PrivateJetModeConst, utils.SidecarModeConst))
	//addApiCmd.Flags().StringArrayVarP(&flagEnv, "env", "e", []string{},
	//	"Environment variables to be passed to deployment")
	//addApiCmd.Flags().StringVarP(&flagImage, "image", "i", "",
	//	"Image of the API. If specified, ignores the value of --override")
	//addApiCmd.Flags().StringVarP(&flagHostname, "hostname", "", "",
	//	"Ingress hostname that the API is being exposed")

	_ = addApiCmd.MarkFlagRequired("name")
	_ = addApiCmd.MarkFlagRequired("from-file")
}

