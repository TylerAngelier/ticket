Feature: Ticket Listing
  As a user
  I want to list tickets in various ways
  So that I can see what work needs to be done

  Background:
    Given a clean tickets directory

  Scenario: List all tickets
    Given a ticket exists with ID "list-0001" and title "First ticket"
    And a ticket exists with ID "list-0002" and title "Second ticket"
    When I run "ticket ls"
    Then the command should succeed
    And the output should contain "list-0001"
    And the output should contain "list-0002"

  Scenario: List command alias works
    Given a ticket exists with ID "list-0001" and title "First ticket"
    When I run "ticket list"
    Then the command should succeed
    And the output should contain "list-0001"

  Scenario: List shows ticket format correctly
    Given a ticket exists with ID "list-0001" and title "My ticket"
    When I run "ticket ls"
    Then the command should succeed
    And the output should match pattern "list-0001\s+\[P2\]\[open\]\s+My ticket"

  Scenario: List with status filter
    Given a ticket exists with ID "list-0001" and title "Open ticket"
    And a ticket exists with ID "list-0002" and title "Closed ticket"
    And ticket "list-0002" has status "closed"
    When I run "ticket ls --status=open"
    Then the command should succeed
    And the output should contain "list-0001"
    And the output should not contain "list-0002"

  Scenario: List nests children under parents
    Given a ticket exists with ID "epi-0001" and title "Epic"
    And a ticket exists with ID "sto-0002" and title "Story"
    And ticket "sto-0002" depends on "epi-0001"
    When I run "ticket ls"
    Then the command should succeed
    And the output line 1 should contain "epi-0001"
    And the output line 2 should contain "sto-0002"

  Scenario: List collapses fully closed subtrees
    Given a ticket exists with ID "epi-0001" and title "Epic"
    And a ticket exists with ID "sto-0002" and title "Story"
    And ticket "sto-0002" depends on "epi-0001"
    And ticket "epi-0001" has status "closed"
    And ticket "sto-0002" has status "closed"
    When I run "ticket ls"
    Then the command should succeed
    And the output should contain "▸ epi-0001"
    And the output should contain "· 2 closed"
    And the output should not contain "sto-0002"

  Scenario: List keeps open parents expanded
    Given a ticket exists with ID "epi-0001" and title "Epic"
    And a ticket exists with ID "sto-0002" and title "Story"
    And ticket "sto-0002" depends on "epi-0001"
    When I run "ticket ls"
    Then the command should succeed
    And the output should not contain "▸ epi-0001"

  Scenario: List shows unmatched parent as context under filter
    Given a ticket exists with ID "epi-0001" and title "Epic"
    And a ticket exists with ID "tsk-0002" and title "Task"
    And ticket "tsk-0002" depends on "epi-0001"
    And ticket "tsk-0002" has status "in_progress"
    When I run "ticket ls --status=in_progress"
    Then the command should succeed
    And the output should contain "(context)"
    And the output should contain "epi-0001"
    And the output should contain "in_progress"

  Scenario: List hides unrelated tickets under filter
    Given a ticket exists with ID "a-0001" and title "Matching task"
    And a ticket exists with ID "b-0002" and title "Other task"
    And ticket "a-0001" has status "in_progress"
    When I run "ticket ls --status=in_progress"
    Then the command should succeed
    And the output should not contain "b-0002"

  Scenario: List shows missing dependencies as second line
    Given a ticket exists with ID "tsk-0001" and title "Task with ghost dep"
    And ticket "tsk-0001" depends on "gho-9999"
    When I run "ticket ls"
    Then the command should succeed
    And the output should contain "(missing)"

  Scenario: List reports dependency cycles with warning
    Given a ticket exists with ID "cyc-0005" and title "Cycle A"
    And a ticket exists with ID "cyc-0006" and title "Cycle B"
    And ticket "cyc-0005" depends on "cyc-0006"
    And ticket "cyc-0006" depends on "cyc-0005"
    When I run "ticket ls"
    Then the command should succeed
    And the output should contain "Warning: dependency cycle detected"
    And the output should contain "⟲"

  Scenario: List renders each multi-parent child once
    Given a ticket exists with ID "p-0001" and title "Parent one"
    And a ticket exists with ID "p-0002" and title "Parent two"
    And a ticket exists with ID "c-0003" and title "Child"
    And ticket "c-0003" depends on "p-0001"
    And ticket "c-0003" depends on "p-0002"
    When I run "ticket ls"
    Then the command should succeed
    And the output line count should be 3

  Scenario: Flat format lists one line per ticket
    Given a ticket exists with ID "epi-0001" and title "Epic"
    And a ticket exists with ID "sto-0002" and title "Story"
    And ticket "sto-0002" depends on "epi-0001"
    When I run "ticket ls --flat"
    Then the command should succeed
    And the output line count should be 2

  Scenario: List with no tickets returns nothing
    When I run "ticket ls"
    Then the output should be empty

  Scenario: Ready shows tickets with no deps and with closed deps
    Given a ticket exists with ID "ready-001" and title "Ready ticket"
    And a ticket exists with ID "ready-002" and title "Unblocked ticket"
    And a ticket exists with ID "ready-003" and title "Dependency"
    And ticket "ready-002" depends on "ready-003"
    And ticket "ready-003" has status "closed"
    When I run "ticket ready"
    Then the command should succeed
    And the output should contain "ready-001"
    And the output should contain "ready-002"

  Scenario: Ready excludes tickets with unclosed deps
    Given a ticket exists with ID "ready-001" and title "Blocked ticket"
    And a ticket exists with ID "ready-002" and title "Open dependency"
    And ticket "ready-001" depends on "ready-002"
    When I run "ticket ready"
    Then the command should succeed
    And the output should not contain "ready-001"
    And the output should contain "ready-002"

  Scenario: Ready shows tickets when deps are closed
    Given a ticket exists with ID "ready-001" and title "Main ticket"
    And a ticket exists with ID "ready-002" and title "Closed dependency"
    And ticket "ready-001" depends on "ready-002"
    And ticket "ready-002" has status "closed"
    When I run "ticket ready"
    Then the command should succeed
    And the output should contain "ready-001"

  Scenario: Ready excludes closed tickets
    Given a ticket exists with ID "ready-001" and title "Closed ticket"
    And ticket "ready-001" has status "closed"
    When I run "ticket ready"
    Then the command should succeed
    And the output should not contain "ready-001"

  Scenario: Ready shows priority in output
    Given a ticket exists with ID "ready-001" and title "Priority ticket"
    When I run "ticket ready"
    Then the command should succeed
    And the output should match pattern "ready-001\s+\[P2\]\[open\]\s+-\s+Priority ticket"

  Scenario: Ready sorts by priority then ID
    Given a ticket exists with ID "ready-003" and title "Low priority" with priority 3
    And a ticket exists with ID "ready-001" and title "High priority" with priority 1
    And a ticket exists with ID "ready-002" and title "Also high priority" with priority 1
    When I run "ticket ready"
    Then the command should succeed
    And the output line 1 should contain "ready-001"
    And the output line 2 should contain "ready-002"
    And the output line 3 should contain "ready-003"

  Scenario: Blocked shows tickets with unclosed deps
    Given a ticket exists with ID "block-001" and title "Blocked ticket"
    And a ticket exists with ID "block-002" and title "Blocker ticket"
    And ticket "block-001" depends on "block-002"
    When I run "ticket blocked"
    Then the command should succeed
    And the output should contain "block-001"
    And the output should contain "<- [block-002]"

  Scenario: Blocked excludes tickets with all deps closed
    Given a ticket exists with ID "block-001" and title "Unblocked ticket"
    And a ticket exists with ID "block-002" and title "Closed blocker"
    And ticket "block-001" depends on "block-002"
    And ticket "block-002" has status "closed"
    When I run "ticket blocked"
    Then the command should succeed
    And the output should not contain "block-001"

  Scenario: Blocked excludes closed tickets
    Given a ticket exists with ID "block-001" and title "Closed blocked"
    And a ticket exists with ID "block-002" and title "Blocker"
    And ticket "block-001" depends on "block-002"
    And ticket "block-001" has status "closed"
    When I run "ticket blocked"
    Then the command should succeed
    And the output should not contain "block-001"

  Scenario: Blocked shows only unclosed blockers
    Given a ticket exists with ID "block-001" and title "Blocked ticket"
    And a ticket exists with ID "block-002" and title "Open blocker"
    And a ticket exists with ID "block-003" and title "Closed blocker"
    And ticket "block-001" depends on "block-002"
    And ticket "block-001" depends on "block-003"
    And ticket "block-003" has status "closed"
    When I run "ticket blocked"
    Then the command should succeed
    And the output should contain "<- [block-002]"
    And the output should not contain "block-003"

  Scenario: Closed shows recently closed tickets
    Given a ticket exists with ID "done-0001" and title "Done ticket"
    And ticket "done-0001" has status "closed"
    When I run "ticket closed"
    Then the command should succeed
    And the output should contain "done-0001"
    And the output should contain "[closed]"
    And the output should contain "Done ticket"

  Scenario: Closed respects limit
    Given a ticket exists with ID "done-0001" and title "First done"
    And a ticket exists with ID "done-0002" and title "Second done"
    And ticket "done-0001" has status "closed"
    And ticket "done-0002" has status "closed"
    When I run "ticket closed --limit=1"
    Then the command should succeed
    And the output line count should be 1

  Scenario: Closed excludes open tickets
    Given a ticket exists with ID "done-0001" and title "Open ticket"
    When I run "ticket closed"
    Then the command should succeed
    And the output should not contain "done-0001"
