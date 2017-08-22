@test

Feature: Just a test! :)

    Scenario: Version
        When executing "minishift version" succeeds
        Then exitcode should equal "0"

    Scenario: Config
        When executing "minishift config view" succeeds
        Then exitcode should not contain "1"

    Scenario: Fail
        When executing "minishift configaro" fails
        Then exitcode should not contain "0"
        Then exitcode should equal "1"