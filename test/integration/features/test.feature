@test
Feature: Terminal test

    Scenario: Start
        When executing "minishift start" succeeds
        Then Minishift should have state "Running"

    Scenario: Docker should not be ready
    This should fail.
        When executing command "docker ps"

    Scenario: Getting values from docker-env command
        When setting scenario variable "dockerenvstdout" to the stdout from executing "minishift docker-env"
        Then exit code should equal "0"
         And stderr should be empty

    Scenario: Executing output from docker-env
       Given user starts terminal instance
        When executing command "minishift $dockerenvstdout"
        When executing command "date"
        When executing command "sleep 10"

    Scenario: Is docker ready?
    Now it should work!
        When executing command "docker ps"

    Scenario: Testing the command which will fail
        When executing command "thiswillfail"

    Scenario: Closing the interactive terminal instance
    All environmental variables will be lost.
        Then user closes terminal instance




    Scenario: Deleting Minishift instance
        Then executing "minishift delete" succeeds