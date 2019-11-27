/*
Copyright 2019 tommylikehu@gmail.com.

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

package controllers

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	csv1alpha1 "github.com/tommylike/code-server-operator/api/v1alpha1"
)

// CodeServerReconciler reconciles a CodeServer object
type CodeServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cs.tommylike.com,resources=codeservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cs.tommylike.com,resources=codeservers/status,verbs=get;update;patch

func (r *CodeServerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	reqLogger := r.Log.WithValues("codeserver", req.NamespacedName)

	// Fetch the CodeServer instance
	codeServer := &csv1alpha1.CodeServer{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, codeServer)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("CodeServer resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get CoderServer.")
		return reconcile.Result{}, err
	}

	if HasCondition(codeServer.Status, csv1alpha1.ServerInactive) {
		if err := r.deleteCodeServerResource(codeServer, false); err != nil {
			return reconcile.Result{Requeue:true}, err
		}
	} else if HasCondition(codeServer.Status, csv1alpha1.ServerRecycled) {
		if err := r.deleteCodeServerResource(codeServer, true); err != nil {
			return reconcile.Result{Requeue:true}, err
		}
	} else {
		// 1/5: reconcile PVC
		_, err := r.reconcileForPVC(codeServer)
		if err != nil {
			return reconcile.Result{Requeue:true}, err
		}
		// 2/5:reconcile ingress
		_, err = r.reconcileForIngress(codeServer)
		if err != nil {
			return reconcile.Result{Requeue:true}, err
		}
		// 3/5: reconcile service
		_, err = r.reconcileForService(codeServer)
		if err != nil {
			return reconcile.Result{Requeue:true}, err
		}
		// 4/5: reconcile deployment
		dep, err := r.reconcileForDeployment(codeServer)
		if err != nil {
			return reconcile.Result{Requeue: true}, err
		}
		// 5/5: update code server status
		if HasCondition(codeServer.Status, csv1alpha1.ServerCreated) {
			createdCondition := NewStateCondition(csv1alpha1.ServerCreated,
				"code server has been accepted", "")
			SetCondition(&codeServer.Status, createdCondition)
		}
		if HasDeploymentCondition(dep.Status, appsv1.DeploymentAvailable) && !HasCondition(codeServer.Status, csv1alpha1.ServerReady) {
			createdCondition := NewStateCondition(csv1alpha1.ServerReady,
				"code server now available", "")
			SetCondition(&codeServer.Status, createdCondition)
		}
		err = r.Client.Update(context.TODO(), codeServer)
		if err != nil {
			reqLogger.Error(err, "Failed to update code server status.", "CodeServer.Namespace", codeServer.Namespace, "CodeServer.Name", codeServer.Name)
			return reconcile.Result{Requeue:true}, nil
		}
	}
	return reconcile.Result{}, nil
}

func (r *CodeServerReconciler) deleteCodeServerResource(codeServer *csv1alpha1.CodeServer, includePVC bool) error {
	reqLogger := r.Log.WithValues("Request.Namespace", codeServer.Namespace, "Request.Name", codeServer.Name)
	reqLogger.Info("Deleting code server resources.")
	//delete ingress
	ing := &extv1.Ingress{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, ing)
	//error of getting object is ignored
	if err == nil {
		err = r.Client.Delete(context.TODO(), ing)
		if err != nil {
			return err
		}
		reqLogger.Info("ingress resource has been successfully deleted.")
	} else if !errors.IsNotFound(err) {
		reqLogger.Info(fmt.Sprintf("failed to get ingress resource for deletion: %v", err))
	}
	//delete service
	srv := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, srv)
	//error of getting object is ignored
	if err == nil {
		err = r.Client.Delete(context.TODO(), srv)
		if err != nil {
			return err
		}
		reqLogger.Info("service resource has been successfully deleted.")
	} else if !errors.IsNotFound(err) {
		reqLogger.Info(fmt.Sprintf("failed to get service resource for deletion: %v", err))
	}
	//delete deployment
	app := &appsv1.Deployment{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, app)
	//error of getting object is ignored
	if err == nil {
		err = r.Client.Delete(context.TODO(), app)
		if err != nil {
			return err
		}
		reqLogger.Info("development resource has been successfully deleted.")
	} else if !errors.IsNotFound(err) {
		reqLogger.Info(fmt.Sprintf("failed to get development resource for deletion: %v", err))
	}
	if includePVC {
		//delete pvc
		pvc := &corev1.PersistentVolumeClaim{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, pvc)
		//error of getting object is ignored
		if err == nil {
			err = r.Client.Delete(context.TODO(), pvc)
			if err != nil {
				return err
			}
			reqLogger.Info("PVC resource has been successfully deleted.")
		} else if !errors.IsNotFound(err) {
			reqLogger.Info(fmt.Sprintf("failed to get PVC resource for deletion: %v", err))
		}
	}
	return nil
}

func (r *CodeServerReconciler) reconcileForPVC(codeServer *csv1alpha1.CodeServer) (*corev1.PersistentVolumeClaim, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", codeServer.Namespace, "Request.Name", codeServer.Name)
	reqLogger.Info("Reconciling persistent volume claim for code server.")
	//reconcile pvc for code server
	newPvc, err := r.pvcForCodeServer(codeServer)
	if err != nil {
		reqLogger.Error(err, "Failed to create new PersistentVolumeClaim.", "PersistentVolumeClaim.Namespace", codeServer.Namespace, "PersistentVolumeClaim.Name", codeServer.Name)
		return nil, err
	}
	oldPvc := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, oldPvc)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a PersistentVolumeClaim.", "PersistentVolumeClaim.Namespace", codeServer.Namespace, "PersistentVolumeClaim.Name", codeServer.Name)
		err = r.Client.Create(context.TODO(), newPvc)
		if err != nil {
			reqLogger.Error(err, "Failed to create PersistentVolumeClaim.", "PersistentVolumeClaim.Namespace", codeServer.Namespace, "PersistentVolumeClaim.Name", codeServer.Name)
			return nil, err
		}
	}else {
		if err != nil {
			//Reschedule the event
			reqLogger.Error(err, fmt.Sprintf("Failed to get PVC for %s.", codeServer.Name))
			return nil, err
		}
		if !equality.Semantic.DeepEqual(oldPvc.Spec, newPvc.Spec){
			oldPvc.Spec = newPvc.Spec
			reqLogger.Info("Updating a PersistentVolumeClaim.", "PersistentVolumeClaim.Namespace", codeServer.Namespace, "PersistentVolumeClaim.Name", codeServer.Name)
			err = r.Client.Update(context.TODO(), oldPvc)
			if err != nil {
				reqLogger.Error(err, "Failed to update PersistentVolumeClaim.", "PersistentVolumeClaim.Namespace", codeServer.Namespace, "PersistentVolumeClaim.Name", codeServer.Name)
				return nil, err
			}
		}
	}
	return oldPvc, nil
}

func (r *CodeServerReconciler) reconcileForDeployment(codeServer *csv1alpha1.CodeServer) (*appsv1.Deployment, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", codeServer.Namespace, "Request.Name", codeServer.Name)
	reqLogger.Info("Reconciling Deployment for code server.")
	//reconcile pvc for code server
	newDev := r.deploymentForCodeServer(codeServer)
	oldDev := &appsv1.Deployment{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, oldDev)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a Deployment.", "Deployment.Namespace", codeServer.Namespace, "Deployment.Name", codeServer.Name)
		err = r.Client.Create(context.TODO(), newDev)
		if err != nil {
			reqLogger.Error(err, "Failed to create Deployment.", "Deployment.Namespace", codeServer.Namespace, "Deployment.Name", codeServer.Name)
			return nil, err
		}
	}else {
		if err != nil {
			//Reschedule the event
			reqLogger.Error(err, fmt.Sprintf("Failed to get Deployment for %s.", codeServer.Name))
			return nil, err
		}
		if !equality.Semantic.DeepEqual(oldDev.Spec, newDev.Spec){
			oldDev.Spec = newDev.Spec
			reqLogger.Info("Updating a Development.", "Deployment.Namespace", codeServer.Namespace, "Deployment.Name", codeServer.Name)
			err = r.Client.Update(context.TODO(), oldDev)
			if err != nil {
				reqLogger.Error(err, "Failed to update Deployment.", "Deployment.Namespace", codeServer.Namespace, "Deployment.Name", codeServer.Name)
				return nil, err
			}
		}
	}
	return oldDev, nil
}

func (r *CodeServerReconciler) reconcileForIngress(codeServer *csv1alpha1.CodeServer) (*extv1.Ingress, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", codeServer.Namespace, "Request.Name", codeServer.Name)
	reqLogger.Info("Reconciling ingress for code server.")
	//reconcile ingress for code server
	newIngress := r.ingressForCodeServer(codeServer)
	oldIngress := &extv1.Ingress{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, oldIngress)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a Ingress.", "Ingress.Namespace", codeServer.Namespace, "Ingress.Name", codeServer.Name)
		err = r.Client.Create(context.TODO(), newIngress)
		if err != nil {
			reqLogger.Error(err, "Failed to create Ingress.", "Ingress.Namespace", codeServer.Namespace, "Ingress.Name", codeServer.Name)
			return nil, err
		}
		// if update is required
	}else {
		if err != nil {
			//Reschedule the event
			reqLogger.Error(err, fmt.Sprintf("Failed to get Ingress for %s.", codeServer.Name))
			return nil, err
		}
		if !equality.Semantic.DeepEqual(oldIngress.Spec, newIngress.Spec) {
			oldIngress.Spec = newIngress.Spec
			reqLogger.Info("Updating a Ingress.", "Ingress.Namespace", codeServer.Namespace, "Ingress.Name", codeServer.Name)
			err = r.Client.Update(context.TODO(), oldIngress)
			if err != nil {
				reqLogger.Error(err, "Failed to update Ingress.", "Ingress.Namespace", codeServer.Namespace, "Ingress.Name", codeServer.Name)
				return nil, err
			}
		}
	}
	return oldIngress, nil
}

func (r *CodeServerReconciler) reconcileForService(codeServer *csv1alpha1.CodeServer) (*corev1.Service, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", codeServer.Namespace, "Request.Name", codeServer.Name)
	reqLogger.Info("Reconciling Service for code server.")
	//reconcile service for code server
	newService := r.serviceForCodeServer(codeServer)
	oldService := &corev1.Service{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: codeServer.Name, Namespace: codeServer.Namespace}, oldService)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a Service.", "Service.Namespace", codeServer.Namespace, "Service.Name", codeServer.Name)
		err = r.Client.Create(context.TODO(), newService)
		if err != nil {
			reqLogger.Error(err, "Failed to create Service.", "Service.Namespace", codeServer.Namespace, "Service.Name", codeServer.Name)
			return nil, err
		}
		// if update is required
	}else {
		if err != nil {
			//Reschedule the event
			reqLogger.Error(err, fmt.Sprintf("Failed to get Service for %s.", codeServer.Name))
			return nil, err
		}
		if !equality.Semantic.DeepEqual(oldService.Spec, newService.Spec) {
			oldService.Spec = newService.Spec
			reqLogger.Info("Updating a Service.", "Service.Namespace", codeServer.Namespace, "Service.Name", codeServer.Name)
			err = r.Client.Update(context.TODO(), oldService)
			if err != nil {
				reqLogger.Error(err, "Failed to update Service.", "Service.Namespace", codeServer.Namespace, "Service.Name", codeServer.Name)
				return nil, err
			}
		}
	}
	return oldService, nil
}

// deploymentForCodeServer returns a code server Deployment object
func (r *CodeServerReconciler) deploymentForCodeServer(m *csv1alpha1.CodeServer) *appsv1.Deployment {
	ls := labelsForCodeServer(m.Name)
	replicas := int32(1)
	enablePriviledge := true
	priviledged := corev1.SecurityContext{
		Privileged: &enablePriviledge,
	}
	shareVolume := corev1.EmptyDirVolumeSource{
		Medium:"",
		SizeLimit:resourcev1.NewQuantity(500, ""),
	}
	dataVolume := corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: pvcNameForCodeServer(m),
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:   m.Spec.Image,
							Name:    "code-server",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env: []corev1.EnvVar{
								{
									Name:  "PASSWORD",
									Value: m.Spec.ServerCipher,
								},
							},
							SecurityContext:&priviledged,
							VolumeMounts:[]corev1.VolumeMount{
								{
									Name:      "/home/coder/.local/share/code-server",
									MountPath: "code-server-share-dir",
								},
								{
									Name:      "/home/coder/project",
									MountPath: "code-server-project-dir",
								},
							},
							Ports: []corev1.ContainerPort{{
								ContainerPort: 8080,
								Name:          "serverhttpport",
							}},
						},
						{
							Image:   "tommylike/code-server-exporter:0.0.1",
							Name:    "status-exporter",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts:[]corev1.VolumeMount{
								{
									Name:      "/home/coder/.local/share/code-server",
									MountPath: "code-server-share-dir",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "STAT_FILE",
									Value: "/home/coder/.local/share/code-server/heartbeat",
								},
								{
									Name:  "LISTEN_PORT",
									Value: "8000",
								},
							},
							Ports: []corev1.ContainerPort{{
								ContainerPort: 8000,
								Name:          "statusreporter",
							}},
						},
					},
					Volumes:[]corev1.Volume{
						{
							Name: "code-server-share-dir",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &shareVolume,
							},
						},
						{
							Name: "code-server-project-dir",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &dataVolume,
							},
						},
					},
				},
			},
		},
	}
	// Set CodeServer instance as the owner of the Deployment.
	controllerutil.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// serviceForCodeServer function takes in a CodeServer object and returns a Service for that object.
func (r *CodeServerReconciler) serviceForCodeServer(m *csv1alpha1.CodeServer) *corev1.Service {
	ls := labelsForCodeServer(m.Name)
	ser := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: ls,
			Ports: []corev1.ServicePort{
				{
					Port: 80,
					Name: "web-ui",
					TargetPort: intstr.FromInt(8080),
				},
				{
					Port: 8000,
					Name: "web-status",
					TargetPort: intstr.FromInt(8000),
				},
			},
		},
	}
	// Set CodeServer instance as the owner of the Service.
	controllerutil.SetControllerReference(m, ser, r.Scheme)
	return ser
}

// pvcForCodeServer function takes in a CodeServer object and returns a PersistentVolumeClaim for that object.
func (r *CodeServerReconciler) pvcForCodeServer(m *csv1alpha1.CodeServer) (*corev1.PersistentVolumeClaim, error) {
	pvcQuantity, err := resourcev1.ParseQuantity(m.Spec.VolumeSize)
	if err != nil  {
		return nil, err
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &m.Spec.StorageClassName,
			Resources: corev1.ResourceRequirements{
				Requests:corev1.ResourceList{
					corev1.ResourceStorage: pvcQuantity,
				},
			},
		},
	}
	// Set CodeServer instance as the owner of the pvc.
	controllerutil.SetControllerReference(m, pvc, r.Scheme)
	return pvc, nil
}

// ingressForCodeServer function takes in a CodeServer object and returns a ingress for that object.
func (r *CodeServerReconciler) ingressForCodeServer(m *csv1alpha1.CodeServer) *extv1.Ingress {
	httpValue := extv1.HTTPIngressRuleValue{
		Paths:[]extv1.HTTPIngressPath{
			{
				Path: fmt.Sprintf("/%s(/|$)(.*)", m.Spec.URL),
				Backend: extv1.IngressBackend{
					ServiceName: m.Name,
					ServicePort: intstr.FromInt(80),
				},
			},
		},
	}
	ingress := &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
			Annotations: annotationsForIngress(m),
		},
		Spec:extv1.IngressSpec{
			Rules:[]extv1.IngressRule{
				{
					//TODO: use configuration to setting this value
					Host: "tommylike.me",
					IngressRuleValue : extv1.IngressRuleValue{
						HTTP: &httpValue,
					},
				},
			},
		},
	}
	// Set CodeServer instance as the owner of the ingress.
	controllerutil.SetControllerReference(m, ingress, r.Scheme)
	return ingress
}

func annotationsForIngress(m *csv1alpha1.CodeServer) map[string]string {
	snippet := fmt.Sprintf(`proxy_set_header Accept-Encoding '';
sub_filter '<head>' '<head> <base href="/%s/">';`, m.Spec.URL)
	return map[string]string{
		"kubernetes.io/ingress.class": "nginx",
		"nginx.ingress.kubernetes.io/use-regex": "true",
		"nginx.ingress.kubernetes.io/rewrite-target": "/$2",
		"nginx.ingress.kubernetes.io/configuration-snippet": snippet,
	}
}



// labelsForCodeServer returns the labels for selecting the resources
// belonging to the given CodeServer name.
func labelsForCodeServer(name string) map[string]string {
	return map[string]string{"app": "codeserver", "cs_name": name}
}

func pvcNameForCodeServer(m *csv1alpha1.CodeServer) string {
	return fmt.Sprintf("%s-pvc", m.Name)
}

// NewStateCondition creates a new code server condition.
func NewStateCondition(conditionType csv1alpha1.ServerConditionType, reason, message string) csv1alpha1.ServerCondition {
	return csv1alpha1.ServerCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// HasCondition checks whether the job has the specified condition
func HasCondition(status csv1alpha1.CodeServerStatus, condType csv1alpha1.ServerConditionType) bool {
	for _, condition := range status.Conditions {
		if condition.Type == condType && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func HasDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) bool {
	for _, condition := range status.Conditions {
		if condition.Type == condType && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// GetCondition gets the condition with specified condition type
func GetCondition(status csv1alpha1.CodeServerStatus, condType csv1alpha1.ServerConditionType) *csv1alpha1.ServerCondition {
	for _, condition := range status.Conditions {
		if condition.Type == condType {
			return &condition
		}
	}
	return nil
}

// SetCondition updates the code server status with provided condition
func SetCondition(status *csv1alpha1.CodeServerStatus, condition csv1alpha1.ServerCondition) {

	currentCond := GetCondition(*status, condition.Type)

	// Do nothing if condition doesn't change
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	// Do not update lastTransitionTime if the status of the condition doesn't change.
	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}

	// Append the updated condition to the job status
	newConditions := filterOutCondition(status, condition)
	status.Conditions = append(newConditions, condition)
}

func filterOutCondition(states *csv1alpha1.CodeServerStatus, currentCondition csv1alpha1.ServerCondition) []csv1alpha1.ServerCondition {

	var newConditions []csv1alpha1.ServerCondition
	for _, condition := range states.Conditions {
		//Filter out the same condition
		if condition.Type == currentCondition.Type {
			continue
		}

		if currentCondition.Type == csv1alpha1.ServerCreated {
			break
		}

		if currentCondition.Type == csv1alpha1.ServerInactive || currentCondition.Type == csv1alpha1.ServerRecycled{
			if currentCondition.Status == corev1.ConditionTrue && condition.Type == csv1alpha1.ServerReady {
				condition.Status = corev1.ConditionFalse
				condition.LastUpdateTime = metav1.Now()
			}
		}

		if currentCondition.Type == csv1alpha1.ServerReady && currentCondition.Status == corev1.ConditionTrue {
			if condition.Type == csv1alpha1.ServerRecycled || condition.Type == csv1alpha1.ServerInactive {
				condition.Status = corev1.ConditionFalse
				condition.LastUpdateTime = metav1.Now()
			}
		}
		newConditions = append(newConditions, condition)
	}
	return newConditions
}

func (r *CodeServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&csv1alpha1.CodeServer{}).
		Complete(r)
}
