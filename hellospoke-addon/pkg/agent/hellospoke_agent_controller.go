package agent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ocmv1alpha1 "open-cluster-management.io/addon-contrib/hellospoke-addon/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HelloSpokeController struct {
	spokeClient client.Client
	hubClient   client.Client
	log         logr.Logger
	clusterName string
}

func (c *HelloSpokeController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ocmv1alpha1.HelloSpoke{}).
		Complete(c)
}

func (c *HelloSpokeController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	c.log.Info(fmt.Sprintf("reconciling... %s", req))
	defer c.log.Info(fmt.Sprintf("done reconcile %s", req))

	helloSpoke := ocmv1alpha1.HelloSpoke{}
	err := c.spokeClient.Get(ctx, req.NamespacedName, &helloSpoke)
	switch {
	case errors.IsNotFound(err):
		return ctrl.Result{}, nil
	case err != nil:
		c.log.Error(err, "unable to get HelloSpoke")
		return ctrl.Result{}, err
	}

	hubHelloSpoke := ocmv1alpha1.HelloSpoke{}
	err = c.hubClient.Get(ctx, types.NamespacedName{Namespace: c.clusterName, Name: helloSpoke.Name}, &hubHelloSpoke)
	switch {
	case errors.IsNotFound(err):
		hubHelloSpoke.Name = helloSpoke.Name
		hubHelloSpoke.Namespace = c.clusterName
		hubHelloSpoke.Status = helloSpoke.Status
		if err = c.hubClient.Create(ctx, &hubHelloSpoke); err != nil {
			c.log.Error(err, "unable to create hub HelloSpoke")
			return ctrl.Result{}, err
		}
	case err != nil:
		c.log.Error(err, "unable to get hub HelloSpoke")
		return ctrl.Result{}, err
	}

	hubHelloSpoke.Status = helloSpoke.Status
	err = c.hubClient.Status().Update(ctx, &hubHelloSpoke)
	if err != nil {
		c.log.Error(err, "unable to update hub HelloSpoke")
	}

	return ctrl.Result{}, err
}
