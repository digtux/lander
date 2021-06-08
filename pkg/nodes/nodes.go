package nodes

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"math"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	// hard limit cache for 15sec, expire at 15m
	pkgCache = cache.New(15*time.Second, 15*time.Minute)
)

//// NodeStats is a simple slice/list of deployment pod numbers
//type NodeStats struct {
//	Bad     int `json:"bad"`
//	Good    int `json:"good"`
//	Unknown int `json:"unknown"`
//}

// getAllNodes speaks to the cluster and attempt to pull all raw Nodes
func getAllNodes(
	logger *log.Logger,
	clientSet *kubernetes.Clientset,
) (*v1.NodeList, error) {
	cacheObj := "v1/NodeList"
	cached, found := pkgCache.Get(cacheObj)
	if found {
		return cached.(*v1.NodeList), nil
	}
	objList, err := clientSet.
		CoreV1().
		Nodes().
		List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	logger.Debugf("got all %s from k8s", cacheObj)
	pkgCache.Set(cacheObj, objList, cache.DefaultExpiration)
	return objList, err
}

func AssembleNodesPieChart(
	logger *log.Logger,
	clientSet *kubernetes.Clientset,
) ([]NodeStats, error) {
	var results []NodeStats //nolint:prealloc
	nodez, err := getAllNodes(logger, clientSet)
	if err != nil {
		logger.Fatal(err)
	}
	now := time.Now()

	// TODO: parse readiness for piechart labels
	for _, i := range nodez.Items {
		age := now.Sub(i.CreationTimestamp.Time)
		newNode := NodeStats{
			AgeSeconds: intergerOnly(age.Seconds()),
			AgeHuman:   humaniseDuration(age),
			Ready:      isReady(i),
			Name:       i.Name,
		}
		results = append(results, newNode)
	}
	return results, err
}

func AssembleNodeTable(
	logger *log.Logger,
	clientSet *kubernetes.Clientset,
	labelSlices []string,
) ([]NodeStats, error) {
	var results []NodeStats //nolint:prealloc
	nodez, err := getAllNodes(logger, clientSet)
	if err != nil {
		logger.Fatal(err)
	}
	now := time.Now()
	for _, i := range nodez.Items {
		age := now.Sub(i.CreationTimestamp.Time)

		matchedLabels := map[string]string{}

		for _, desiredLabel := range labelSlices {
			humanKey := shortLabelName(desiredLabel)
			x := getLabelValue(i, desiredLabel)
			matchedLabels[humanKey] = x
		}

		newNode := NodeStats{
			AgeSeconds: intergerOnly(age.Seconds()),
			AgeHuman:   humaniseDuration(age),
			Ready:      isReady(i),
			Name:       i.Name,
			LabelMap:   matchedLabels,
		}
		results = append(results, newNode)
	}
	return results, err
}

func shortLabelName(input string) string {
	// shortens labels like:
	//  - node.kubernetes.io/instance-type  => instance-type
	//  - node.kubernetes.io/instancegroup  => instancegroup
	//  - topology.kubernetes.io/zone       => zone
	// This basically grabs whatever comes after the final `/`
	splitString := strings.Split(input, "/")
	// wasn't split:
	if len(splitString) < 1 {
		return input
	}
	// return the last slice
	return splitString[len(splitString)-1]
}

func getLabelValue(node v1.Node, label string) string {
	for k, v := range node.Labels {
		if k == label {
			return v
		}
	}
	return ""
}

// convert node ages into something similar to that in kubectl get nodes
func humaniseDuration(duration time.Duration) string {
	// most cases.. its older than a dat, we'll round it to days
	if duration > (24 * time.Hour) {
		num := roundTime(duration.Seconds() / 86400)
		return fmt.Sprintf("%dd", num)
	}
	if duration > (1 * time.Hour) {
		num := roundTime(duration.Seconds() / 3600)
		return fmt.Sprintf("%dh", num)
	}
	if duration > (1 * time.Minute) {
		num := roundTime(duration.Seconds() / 60)
		return fmt.Sprintf("%dm", num)
	}
	seconds := intergerOnly(duration.Seconds())
	return fmt.Sprintf("%ds", seconds)
}

func roundTime(input float64) int {
	// credit: https://www.socketloop.com/tutorials/golang-get-time-duration-in-year-month-week-or-day
	var result float64
	if input < 0 {
		result = math.Ceil(input - 0.5)
	} else {
		result = math.Floor(input + 0.5)
	}
	return intergerOnly(result)
}

func intergerOnly(input float64) int {
	i, _ := math.Modf(input)
	return int(i)
}

func isReady(k8sObject v1.Node) bool {
	for _, conditions := range k8sObject.Status.Conditions {
		if strings.Contains(string(conditions.Type), "Ready") {
			if strings.Contains(string(conditions.Status), "True") {
				return true
			}
		}
	}
	return false
}
