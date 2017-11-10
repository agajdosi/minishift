@addon-xpaas @addon
Feature: XpaaS add-on

  Scenario: User enables the xpaas add-on
     When executing "minishift addons enable xpaas" succeeds
     Then stdout should contain "Add-on 'xpaas' enabled"

  Scenario: User starts Minishift
    Given Minishift has state "Does Not Exist"
     When executing "minishift start --memory 4GB" succeeds
     Then Minishift should have state "Running"
      And stdout should contain
      """
      XPaaS OpenShift imagestream and templates installed
      """

  Scenario Outline: User deploys, checks out and deletes several templates from XpaaS imagestream
   Given Minishift has state "Running"
    When executing "oc new-app <template-name>" succeeds
     And services "<service-name>" rollout successfully
    Then body of HTTP request to "<http-endpoint>" of service "<service-name>" in namespace "myproject" contains "<expected-hello>"
     And status code of HTTP request to "<http-endpoint>" of service "<service-name>" in namespace "myproject" is equal to "200"
     And executing "oc delete all --all" succeeds

  Examples: Template names and values to be used with each of them
    | template-name           | service-name   | http-endpoint | expected-hello                        |
    | datagrid65-basic        | datagrid-app   | /             | Welcome to the JBoss Data Grid Server |
    | eap64-basic-s2i         | eap-app        | /index.jsf    | Welcome to JBoss!                     |
    | eap70-basic-s2i         | eap-app        | /index.jsf    | Welcome to JBoss!                     |

  Scenario: User deletes Minishift
     When executing "minishift delete --force" succeeds
     Then Minishift should have state "Does Not Exist"
