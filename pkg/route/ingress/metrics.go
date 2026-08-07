package ingress

import (
	"github.com/blang/semver/v4"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/openshift/route-controller-manager/pkg/routecontroller"
)

var (
	unmanagedRoutes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: routecontroller.MetricRouteWithUnmanagedOwner,
		Help: "Report the number of routes owned by unmanaged ingresses.",
	}, []string{"name", "namespace", "host"})

	ingressesWithoutClassName = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: routecontroller.MetricIngressWithoutClassName,
		Help: "Report the number of ingresses that do not specify ingressClassName.",
	}, []string{"name", "namespace"})
)

func (c *Controller) Create(v *semver.Version) bool {
	c.metricsCreateOnce.Do(func() {
		c.metricsCreateLock.Lock()
		defer c.metricsCreateLock.Unlock()
		c.metricsCreated = true
	})
	return c.MetricsCreated()
}

func (c *Controller) MetricsCreated() bool {
	return c.metricsCreated
}

func (c *Controller) ClearState() {
	c.metricsCreateLock.Lock()
	defer c.metricsCreateLock.Unlock()
	c.metricsCreated = false
}

// FQName returns the fully-qualified metric name of the collector.
func (c *Controller) FQName() string {
	return routecontroller.MetricRouteController
}

func (c *Controller) Describe(ch chan<- *prometheus.Desc) {
	unmanagedRoutes.Describe(ch)
	ingressesWithoutClassName.Describe(ch)
}

func (c *Controller) Collect(ch chan<- prometheus.Metric) {
	// collect ingresses that do not specify ingressClassName
	ingressInstances, err := c.ingressLister.List(labels.Everything())
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	for _, ingressInstance := range ingressInstances {
		labelVal := 0
		icName := ingressInstance.Spec.IngressClassName
		if icName == nil || *icName == "" {
			labelVal = 1
		}
		ingressesWithoutClassName.WithLabelValues(ingressInstance.Name, ingressInstance.Namespace).Set(float64(labelVal))
	}

	ingressesWithoutClassName.Collect(ch)

	// collect routes owned by ingresses no longer managed
	routeInstances, err := c.routeLister.List(labels.Everything())
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	// Cache ingressManaged results keyed by "namespace/name" to avoid
	// redundant lister + ingressclass lookups for ingresses shared by many routes.
	managedCache := make(map[string]bool, len(ingressInstances))

	for _, routeInstance := range routeInstances {
		labelVal := 0
		if ownerName, have := hasIngressOwnerRef(routeInstance.OwnerReferences); have {
			// Owner references are namespace-scoped, so the owning ingress
			// is always in the same namespace as the route.
			cacheKey := routeInstance.Namespace + "/" + ownerName
			managed, cached := managedCache[cacheKey]
			if !cached {
				ingress, err := c.ingressLister.Ingresses(routeInstance.Namespace).Get(ownerName)
				if err == nil && ingress != nil {
					managed, err = c.ingressManaged(ingress)
					if err != nil {
						utilruntime.HandleError(err)
						continue
					}
					managedCache[cacheKey] = managed
				}
			}
			if !managed {
				labelVal = 1
			}
		}
		unmanagedRoutes.WithLabelValues(routeInstance.Name, routeInstance.Namespace, routeInstance.Spec.Host).Set(float64(labelVal))
	}

	unmanagedRoutes.Collect(ch)
}

// ResetIngressMetrics clears metrics for the specified ingress by setting its
// series data to 0.  This is appropriate to do when an ingress object is
// deleted to prevent stale metrics from triggering alerts.  As Collect only
// updates metrics for ingresses that exist at the time when Collect is called,
// it does not clear metrics for deleted routes.
func (c *Controller) ResetIngressMetrics(namespace, ingressName string) {
	ingressesWithoutClassName.WithLabelValues(ingressName, namespace).Set(0.0)
}
