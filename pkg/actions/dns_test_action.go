package actions

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net"
)

type DNSTestAction struct {
	Namespace   string `yaml:"namespace"`
	ServiceName string `yaml:"serviceName"`
	DomainName  string `yaml:"domainName"`
}

func (a *DNSTestAction) Execute(ctx ActionContext) error {

	if a.Namespace == "" {
		return errors.New("namespace is required")
	}

	if a.ServiceName == "" {
		return errors.New("serviceName is required")
	}

	if a.DomainName == "" {
		return errors.New("domainName is required")
	}

	log := ctx.Log()

	var client *kubernetes.Clientset
	err := ctx.Provide(&client)
	if err != nil {
		log.Fatal(err)
	}

	service, err := client.CoreV1().Services(a.Namespace).Get(a.ServiceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(service.Status.LoadBalancer.Ingress) == 0 {
		return errors.New("load balancer not yet provisioned, waiting for load balancer...")
	}

	for _, ingress := range service.Status.LoadBalancer.Ingress {
		expectedIP := ingress.IP
		hostname := ingress.Hostname

		ips, err := net.LookupIP(a.DomainName)
		if err != nil {
			return errors.Errorf(`
--------------
Could not resolve domain name %q. Error: %s

Have you configured a DNS record yet?

The load-balancer is reporting the following values which should be used to configure DNS:

IP: 	  %s
Hostname: %s

Please configure the DNS record before proceeding.
-------------
`, a.DomainName, err, expectedIP, hostname)
		}

		matchedIP := false
		var foundIPs []string
		for _, ip := range ips {
			if ip.String() == expectedIP {
				matchedIP = true
			}
			foundIPs = append(foundIPs, ip.String())
		}
		if !matchedIP {
			return errors.Errorf(`
--------------
Domain name %q resolved to the wrong IP address(es). 

Expected: %s
Found:    %s

Have you configured a DNS record yet?

The load-balancer is reporting the following values which should be used to configure DNS:

IP: 	  %s
Hostname: %s

Please configure the DNS record before proceeding.
-------------
`, a.DomainName, expectedIP, foundIPs, expectedIP, hostname)
		}

		log.Infof("Resolved domain %s to addresses: %+v", a.DomainName, foundIPs)
	}

	return nil
}
