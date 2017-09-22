@future
Feature: Future OCP
  As a user I can perform basic operations of Minishift and OpenShift

  Scenario: Config should be empty
     When executing "minishift config view" succeeds
     Then stdout should be empty

  Scenario: CDK starts with newest internal OCP image
    Given Minishift has state "Does Not Exist"
     When executing "minishift start" succeeds
     Then stdout should contain
      """
      Pulling image brew-pulp-docker01.web.prod.ext.phx2.redhat.com:8888/openshift3/ose:latest
      """

  Scenario: OpenShift has latest version
     When executing "minishift openshift version" succeeds
     Then stdout should contain "openshift v3.6.173.0.40"

  Scenario: Deleting Minishift
    Given Minishift has state "Running"
     When executing "minishift delete --force" succeeds
     Then Minishift should have state "Does Not Exist"
