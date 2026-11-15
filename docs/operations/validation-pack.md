# Validation pack: electronic signatures

Every requirement line of blueprint chapter 09.8 and the tests that prove it (DOC-790). A customer who validates their use of Diffluence takes this list with the results of the release's run: `make check` for the Go tests, `scripts/e2e` for the browser tests. `TestTheValidationPackNamesEveryLineAndItsTests` fails when a line of 09.8 is missing here or a test named here does not exist. What remains the customer's is in `regulated-use.md`.

Go tests are named by their function, browser tests by their title after the word e2e.

| Line | What it asks | Proved by |
|---|---|---|
| DOC-700 | A step can require a signature, of its taker or of every approving reviewer | `TestASignatureFitsAnAuthorsStepAndAnApprovedReview`, `TestASignatureIsRefusedToAnyoneButItsSigner`, `TestEveryApprovingReviewerSignsAndARemovedOneCannot` |
| DOC-710 | The record holds signer, printed name, meaning, time, page, version, workflow version, step and method | `TestASignatureRecordHoldsEveryField` |
| DOC-711 | The record holds the hash of the signed content; any change changes it | `TestAnyChangeToTheSignedContentChangesItsHash`, `TestTheCanonicalDocumentIsStable`, `TestASignatureIsRefusedForAVersionThatChangedAfterItWasConfirmed`, `TestTamperingInTheDatabaseTurnsTheMarkToDoesNotMatch` |
| DOC-712 | The signature and its step are one transaction; the audit entry holds the hash | `TestAFailedStepLeavesNoSignature`, `TestASignatureRecordHoldsEveryField` |
| DOC-720 | The dialog names the meaning before anything else | e2e: "an author signs an approval in the signing dialog", e2e: "the signing dialog names the meaning, the version and its title" |
| DOC-721 | Meanings come from a list a space can extend | `TestASignatureMeaningIsCheckedByTheValidator`, `TestASpaceAddsMeaningsAndAWorkflowNamesOnlyThose`, `TestEveryBuiltInMeaningHasWordsAndAKey` |
| DOC-730 | A fresh confirmation for one signature, two minutes, two components | `TestAConfirmationCountsOnceForOneSignatureWithinTwoMinutes`, `TestAConfirmationThatSignedOnceIsRefusedForTheSameStepAgain`, `TestAPasskeySignsOnlyWithUserVerification` |
| DOC-731 | The identity provider confirms with a login after the step began | `TestTheIdentityProviderMustConfirmAfterTheSigningStepBegan` |
| DOC-732 | Failed confirmations count toward the lockout | `TestFailedSigningConfirmationsCountTowardTheLockout` |
| DOC-733 | No signing through tokens, automation or the system | `TestASignatureIsRefusedToAnyoneButItsSigner` |
| DOC-740 | The page and its history show the signatures | `TestASignatureRecordHoldsEveryField`, `TestThePublishedCopyOfASignedVersionShowsItsSignatures`, e2e: "an author signs an approval in the signing dialog" |
| DOC-741 | Verification recomputes the hash and checks the audit entry | `TestTamperingInTheDatabaseTurnsTheMarkToDoesNotMatch` |
| DOC-742 | The auditor's export verifies outside Diffluence | `TestTheAuditorsExportVerifiesOutsideDiffluence` |
| DOC-750 | A signature belongs to one account; a disabled one's still verify | `TestSignaturesOfADisabledSignerStillVerify` |
| DOC-751 | Signing is enabled per person, with how they were identified | `TestASignatureIsRefusedToAnyoneButItsSigner`, `TestSignaturesOfADisabledSignerStillVerify` |
| DOC-760 | Signing needs the right to take the step | `TestASignatureIsRefusedToAnyoneButItsSigner` |
| DOC-770 | Signatures and signed versions are never deleted or changed | `TestSignedVersionsSurviveDeletePurgeAndRemoval` |
| DOC-771 | Erasure keeps signature records | `TestErasureKeepsSignaturesAndErasesTheRest`, `TestAnonymisingLeavesTheIdOnlyWhereThatIsDecided` |
| DOC-772 | Signatures verify after a restore | `TestTamperingInTheDatabaseTurnsTheMarkToDoesNotMatch` (the check `diffluence signatures verify` runs), and the step in `scripts/restore-drill.sh` |
| DOC-790 | This pack names every line | `TestTheValidationPackNamesEveryLineAndItsTests` |
| DOC-791 | Qualified signatures through a trust provider | Not built: open for the product owner |
