/*
Copyright 2019 The Kubernetes Authors.

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

package kubeletconfig

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"

	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
)

func GetKubeletConfigForNodes(kc *Kubectl, nodeNames []string, logger *log.Logger) (map[string]*kubeletconfigv1beta1.KubeletConfiguration, error) {
	cmd := kc.Command("proxy", "-p", "0")
	stdout, stderr, err := StartWithStreamOutput(cmd)
	defer stdout.Close()
	defer stderr.Close()
	defer cmd.Process.Kill()

	port, err := getKubeletProxyPort(stdout)
	if err != nil {
		return nil, err
	}
	logger.Printf("proxy port: %d", port)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	kubeletConfs := make(map[string]*kubeletconfigv1beta1.KubeletConfiguration)
	for _, nodeName := range nodeNames {
		endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/v1/nodes/%s/proxy/configz", port, nodeName)

		logger.Printf("querying endpoint: %q", endpoint)
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			logger.Printf("request creation failed for %q: %v - skipped", endpoint, err)
			continue
		}
		req.Header.Add("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			logger.Printf("request execution failed for %q: %v - skipped", endpoint, err)
			continue
		}
		if resp.StatusCode != 200 {
			logger.Printf("unexpected response status code for %q: %d - skipped", endpoint, resp.StatusCode)
			continue
		}

		conf, err := decodeConfigz(resp)
		if err != nil {
			logger.Printf("response decode failed for %q: %v - skipped", endpoint, err)
			continue
		}

		kubeletConfs[nodeName] = conf
	}
	return kubeletConfs, nil
}

func getKubeletProxyPort(r io.Reader) (int, error) {
	buf := make([]byte, 128)
	n, err := r.Read(buf)
	if err != nil {
		return -1, err
	}
	output := string(buf[:n])
	proxyRegexp, err := regexp.Compile("Starting to serve on 127.0.0.1:([0-9]+)")
	if err != nil {
		return -1, err
	}
	match := proxyRegexp.FindStringSubmatch(output)
	return strconv.Atoi(match[1])
}

func decodeConfigz(resp *http.Response) (*kubeletconfigv1beta1.KubeletConfiguration, error) {
	type configzWrapper struct {
		ComponentConfig kubeletconfigv1beta1.KubeletConfiguration `json:"kubeletconfig"`
	}

	configz := configzWrapper{}
	contentsBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(contentsBytes, &configz)
	if err != nil {
		return nil, err
	}

	return &configz.ComponentConfig, nil
}
