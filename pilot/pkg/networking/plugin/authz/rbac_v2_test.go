package authz

import (
	"reflect"
	"testing"

	"istio.io/istio/pilot/pkg/networking/plugin/authn"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	policy "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v2alpha"
	metadata "github.com/envoyproxy/go-control-plane/envoy/type/matcher"

	rbacproto "istio.io/api/rbac/v1alpha1"
	"istio.io/istio/pilot/pkg/model"
)

func TestConvertRbacRulesToFilterConfigV2(t *testing.T) {
	roles := []model.Config{
		{
			ConfigMeta: model.ConfigMeta{Name: "service-role-1"},
			Spec: &rbacproto.ServiceRole{
				Rules: []*rbacproto.AccessRule{
					{
						NotMethods: []string{"DELETE"},
					},
				},
			},
		},
		{
			ConfigMeta: model.ConfigMeta{Name: "service-role-2"},
			Spec: &rbacproto.ServiceRole{
				Rules: []*rbacproto.AccessRule{
					{
						NotMethods: []string{"DELETE"},
						Constraints: []*rbacproto.AccessRule_Constraint{
							{Key: "destination.labels[app]", Values: []string{"foo"}},
						},
					},
				},
			},
		},
	}
	bindings := []model.Config{
		{
			ConfigMeta: model.ConfigMeta{Name: "authz-policy-single-binding"},
			Spec: &rbacproto.AuthorizationPolicy{
				Allow: []*rbacproto.ServiceRoleBinding{
					{
						Subjects: []*rbacproto.Subject{
							{
								Names: []string{"allUsers"},
							},
						},
						RoleRef: &rbacproto.RoleRef{
							Kind: "ServiceRole",
							Name: "service-role-1",
						},
					},
				},
			},
		},
		{
			ConfigMeta: model.ConfigMeta{Name: "authz-policy-multiple-bindings-with-selector"},
			Spec: &rbacproto.AuthorizationPolicy{
				WorkloadSelector: &rbacproto.WorkloadSelector{
					Labels: map[string]string{
						"app": "productpage",
					},
				},
				Allow: []*rbacproto.ServiceRoleBinding{
					{
						Subjects: []*rbacproto.Subject{
							{
								Names: []string{"allUsers"},
							},
						},
						RoleRef: &rbacproto.RoleRef{
							Kind: "ServiceRole",
							Name: "service-role-1",
						},
					},
					{
						Subjects: []*rbacproto.Subject{
							{
								Namespaces: []string{"testing"},
							},
						},
						RoleRef: &rbacproto.RoleRef{
							Kind: "ServiceRole",
							Name: "service-role-1",
						},
					},
				},
			},
		},
		{
			ConfigMeta: model.ConfigMeta{Name: "authz-policy-selector-with-no-match"},
			Spec: &rbacproto.AuthorizationPolicy{
				WorkloadSelector: &rbacproto.WorkloadSelector{
					Labels: map[string]string{
						"app": "bar",
					},
				},
				Allow: []*rbacproto.ServiceRoleBinding{
					{
						Subjects: []*rbacproto.Subject{
							{
								Names: []string{"allUsers"},
							},
						},
						RoleRef: &rbacproto.RoleRef{
							Kind: "ServiceRole",
							Name: "service-role-2",
						},
					},
				},
			},
		},
		{
			ConfigMeta: model.ConfigMeta{Name: "authz-policy-single-binding-all-authned-users"},
			Spec: &rbacproto.AuthorizationPolicy{
				Allow: []*rbacproto.ServiceRoleBinding{
					{
						Subjects: []*rbacproto.Subject{
							{
								Names: []string{"allAuthenticatedUsers"},
							},
						},
						RoleRef: &rbacproto.RoleRef{
							Kind: "ServiceRole",
							Name: "service-role-2",
						},
					},
				},
			},
		},
	}

	notDeleteMethodPermissions := []*policy.Permission{
		{
			Rule: &policy.Permission_AndRules{
				AndRules: &policy.Permission_Set{
					Rules: []*policy.Permission{
						{
							Rule: &policy.Permission_NotRule{
								NotRule: &policy.Permission{
									Rule: generateHeaderRule([]*route.HeaderMatcher{
										{Name: methodHeader, HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{ExactMatch: "DELETE"}},
									}),
								},
							},
						},
					},
				},
			},
		},
	}
	anyPrincipals := []*policy.Principal{{
		Identifier: &policy.Principal_AndIds{
			AndIds: &policy.Principal_Set{
				Ids: []*policy.Principal{
					{
						Identifier: &policy.Principal_OrIds{
							OrIds: &policy.Principal_Set{
								Ids: []*policy.Principal{
									{
										Identifier: &policy.Principal_Any{
											Any: true,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}}
	anyAuthenticatedPrincipals := []*policy.Principal{{
		Identifier: &policy.Principal_AndIds{
			AndIds: &policy.Principal_Set{
				Ids: []*policy.Principal{
					{
						Identifier: &policy.Principal_OrIds{
							OrIds: &policy.Principal_Set{
								Ids: []*policy.Principal{
									{
										Identifier: &policy.Principal_Metadata{
											Metadata: generateMetadataStringMatcher(
												attrSrcPrincipal, &metadata.StringMatcher{
													MatchPattern: &metadata.StringMatcher_Regex{
														Regex: ".*",
													}},
												authn.AuthnFilterName),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}}
	testingNamespacePrincipals := []*policy.Principal{
		{
			Identifier: &policy.Principal_AndIds{
				AndIds: &policy.Principal_Set{
					Ids: []*policy.Principal{
						{
							Identifier: &policy.Principal_OrIds{
								OrIds: &policy.Principal_Set{
									Ids: []*policy.Principal{
										{
											Identifier: &policy.Principal_Metadata{
												Metadata: generateMetadataStringMatcher(
													attrSrcPrincipal, &metadata.StringMatcher{
														MatchPattern: &metadata.StringMatcher_Regex{Regex: `.*/ns/testing/.*`}}, authn.AuthnFilterName),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	policy1 := &policy.Policy{
		Permissions: notDeleteMethodPermissions,
		Principals:  anyPrincipals,
	}
	policy2 := &policy.Policy{
		Permissions: notDeleteMethodPermissions,
		Principals:  anyPrincipals,
	}
	policy3 := &policy.Policy{
		Permissions: notDeleteMethodPermissions,
		Principals:  testingNamespacePrincipals,
	}
	policy4 := &policy.Policy{
		Permissions: notDeleteMethodPermissions,
		Principals:  anyAuthenticatedPrincipals,
	}

	expectRbac1 := generateExpectRBACWithAuthzPolicyKeysAndRbacPolicies([]string{
		"authz-policy-authz-policy-single-binding-allow[0]"},
		[]*policy.Policy{policy1})
	expectRbac2 := generateExpectRBACWithAuthzPolicyKeysAndRbacPolicies([]string{
		"authz-policy-authz-policy-multiple-bindings-with-selector-allow[0]",
		"authz-policy-authz-policy-multiple-bindings-with-selector-allow[1]",
		"authz-policy-authz-policy-single-binding-allow[0]"},
		[]*policy.Policy{policy2, policy3, policy1})
	expectRbac3 := generateExpectRBACWithAuthzPolicyKeysAndRbacPolicies([]string{
		"authz-policy-authz-policy-single-binding-all-authned-users-allow[0]",
		"authz-policy-authz-policy-single-binding-allow[0]"},
		[]*policy.Policy{policy4, policy1})

	authzPolicies := newAuthzPoliciesWithRolesAndBindings(roles, bindings)
	option := rbacOption{authzPolicies: authzPolicies}
	option.authzPolicies.IsRbacV2 = true

	testCases := []struct {
		name    string
		service *serviceMetadata
		rbac    *policy.RBAC
		option  rbacOption
	}{
		{
			name: "service with single binding",
			service: &serviceMetadata{
				name: "service-1",
			},
			rbac:   expectRbac1,
			option: option,
		},
		{
			name: "service with multiple bindings and workload selector",
			service: &serviceMetadata{
				name:   "productpage",
				labels: map[string]string{"app": "productpage"},
			},
			rbac:   expectRbac2,
			option: option,
		},
		{
			name: "service no selector matched",
			service: &serviceMetadata{
				name:   "service-2",
				labels: map[string]string{"app": "foo"},
			},
			rbac:   expectRbac3,
			option: option,
		},
	}

	for _, tc := range testCases {
		rbac := convertRbacRulesToFilterConfigV2(tc.service, tc.option)
		if !reflect.DeepEqual(*tc.rbac, *rbac.Rules) {
			t.Errorf("%s want:\n%v\nbut got:\n%v", tc.name, *tc.rbac, *rbac.Rules)
		}
	}
}
