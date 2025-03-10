package tenant

import (
	"context"
	"fmt"
	"strconv"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	capsulev1beta1 "github.com/clastix/capsule/api/v1beta1"
)

// Ensuring all the LimitRange are applied to each Namespace handled by the Tenant.
func (r *Manager) syncLimitRanges(tenant *capsulev1beta1.Tenant) error {
	// getting requested LimitRange keys
	keys := make([]string, 0, len(tenant.Spec.LimitRanges.Items))

	for i := range tenant.Spec.LimitRanges.Items {
		keys = append(keys, strconv.Itoa(i))
	}

	group := new(errgroup.Group)

	for _, ns := range tenant.Status.Namespaces {
		namespace := ns

		group.Go(func() error {
			return r.syncLimitRange(tenant, namespace, keys)
		})
	}

	return group.Wait()
}

func (r *Manager) syncLimitRange(tenant *capsulev1beta1.Tenant, namespace string, keys []string) (err error) {
	// getting LimitRange labels for the mutateFn
	var tenantLabel, limitRangeLabel string

	if tenantLabel, err = capsulev1beta1.GetTypeLabel(&capsulev1beta1.Tenant{}); err != nil {
		return
	}
	if limitRangeLabel, err = capsulev1beta1.GetTypeLabel(&corev1.LimitRange{}); err != nil {
		return
	}

	if err = r.pruningResources(namespace, keys, &corev1.LimitRange{}); err != nil {
		return
	}

	for i, spec := range tenant.Spec.LimitRanges.Items {
		target := &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("capsule-%s-%d", tenant.Name, i),
				Namespace: namespace,
			},
		}

		var res controllerutil.OperationResult
		res, err = controllerutil.CreateOrUpdate(context.TODO(), r.Client, target, func() (err error) {
			target.ObjectMeta.Labels = map[string]string{
				tenantLabel:     tenant.Name,
				limitRangeLabel: strconv.Itoa(i),
			}
			target.Spec = spec
			return controllerutil.SetControllerReference(tenant, target, r.Scheme)
		})

		r.emitEvent(tenant, target.GetNamespace(), res, fmt.Sprintf("Ensuring LimitRange %s", target.GetName()), err)

		r.Log.Info("LimitRange sync result: "+string(res), "name", target.Name, "namespace", target.Namespace)
		if err != nil {
			return
		}
	}

	return
}
