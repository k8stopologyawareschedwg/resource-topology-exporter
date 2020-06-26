package pciconf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	sriovtypes "github.com/intel/sriov-network-device-plugin/pkg/types"

	"github.com/fromanirh/numalign/pkg/topologyinfo/pcidev"
)

// PCIResourceMap is a mapping pci address -> resource name
type PCIResourceMap map[string]string

func GetFromSRIOVConfigFile(sysfsRoot, path string) (PCIResourceMap, error) {
	pciDevs, err := pcidev.NewPCIDevices(sysfsRoot)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("missing configuration data from %q", path)
	}

	return parseSRIOVResourceListFromData(data, pciDevs)
}

func parseSRIOVResourceListFromData(data []byte, pciDevs *pcidev.PCIDevices) (PCIResourceMap, error) {
	var err error

	resources := &sriovtypes.ResourceConfList{}
	err = json.Unmarshal(data, resources)
	if err != nil {
		return nil, err
	}

	pci2Res := make(PCIResourceMap)
	for i := range resources.ResourceList {
		conf := &resources.ResourceList[i]

		resourceName := conf.ResourceName
		if conf.ResourcePrefix != "" {
			resourceName = fmt.Sprintf("%s/%s", conf.ResourcePrefix, conf.ResourceName)
		}

		ndSelectors := &sriovtypes.NetDeviceSelectors{}
		err = json.Unmarshal(*conf.Selectors, ndSelectors)
		if err != nil {
			log.Printf("Error unmarshalling selectors for %q: %v (skipped)", resourceName, err)
			continue
		}

		if len(ndSelectors.Drivers) != 0 {
			log.Printf("Unsupported drivers selector for %q: %v (skipped)", resourceName, ndSelectors.Drivers)
		}

		for _, pciAddrSel := range ndSelectors.PciAddresses {
			log.Printf("PCI %q -> Resource %q", pciAddrSel, resourceName)
			pci2Res[pciAddrSel] = resourceName
		}

		for _, vendorSel := range ndSelectors.Vendors {
			for _, deviceSel := range ndSelectors.Devices {
				pciAddrs := findPCIAddressFromVendorModel(pciDevs, vendorSel, deviceSel)
				if len(pciAddrs) == 0 {
					log.Printf("Cannot find PCI device for %s:%s (skipped)", vendorSel, deviceSel)
					continue
				}

				for _, pciAddr := range pciAddrs {
					log.Printf("PCI %q -> Resource %q", pciAddr, resourceName)
					pci2Res[pciAddr] = resourceName
				}
			}
		}
	}
	return pci2Res, nil
}

func findPCIAddressFromVendorModel(pciDevs *pcidev.PCIDevices, vendor, device string) []string {
	var foundDevs []string
	wantedPCIID := fmt.Sprintf("%s:%s", vendor, device)
	for _, pciDev := range pciDevs.Items {
		currentPCIID := fmt.Sprintf("%04x:%04x", pciDev.Vendor(), pciDev.Device())
		if wantedPCIID == currentPCIID {
			foundDevs = append(foundDevs, pciDev.Address())
		}
	}
	return foundDevs
}
