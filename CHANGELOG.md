# Changelog

## [0.10.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.9.0...tally-v0.10.0) (2026-07-21)


### Features

* add no:parent search filter to show rolled-up sub-notches ([#117](https://github.com/clarkbar-sys/tally/issues/117)) ([9c64189](https://github.com/clarkbar-sys/tally/commit/9c64189d12ca66ef508f6ae1425de8d188678984))
* **web:** detect sync conflicts with version CAS + quarantine view ([#125](https://github.com/clarkbar-sys/tally/issues/125)) ([a3af00e](https://github.com/clarkbar-sys/tally/commit/a3af00eed9c1e369aadea9e082d90bb06cd4e497)), closes [#124](https://github.com/clarkbar-sys/tally/issues/124)
* **web:** give notches an optional due date ([#120](https://github.com/clarkbar-sys/tally/issues/120)) ([baeef65](https://github.com/clarkbar-sys/tally/commit/baeef65d71d46f80f492799f37c02de2be9ecf98)), closes [#119](https://github.com/clarkbar-sys/tally/issues/119)
* **web:** local IndexedDB mirror + hub sync, so edits survive offline ([#122](https://github.com/clarkbar-sys/tally/issues/122)) ([#123](https://github.com/clarkbar-sys/tally/issues/123)) ([87a7438](https://github.com/clarkbar-sys/tally/commit/87a74383839847ba1a9093b905b5126a7644850c))
* **web:** persist the live build to server-side SQLite, replacing IndexedDB ([#121](https://github.com/clarkbar-sys/tally/issues/121)) ([4d22c24](https://github.com/clarkbar-sys/tally/commit/4d22c245e38ba3d6be6e518bd555864a7fe74834)), closes [#113](https://github.com/clarkbar-sys/tally/issues/113)

## [0.9.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.8.0...tally-v0.9.0) (2026-07-21)


### Features

* **web:** show version and offer upgrade when a newer release exists ([#114](https://github.com/clarkbar-sys/tally/issues/114)) ([516a57e](https://github.com/clarkbar-sys/tally/commit/516a57ee117df825cc02a010860ba5919e13cb4f))

## [0.8.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.7.0...tally-v0.8.0) (2026-07-21)


### Features

* **web:** persist on the tailnet, keep demo mode for the static export ([#110](https://github.com/clarkbar-sys/tally/issues/110)) ([54ce37d](https://github.com/clarkbar-sys/tally/commit/54ce37de3989a8e3b22028304a832b3721828929))

## [0.7.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.6.0...tally-v0.7.0) (2026-07-21)


### Features

* **deploy:** install tally from prebuilt release binaries, like hush ([#108](https://github.com/clarkbar-sys/tally/issues/108)) ([18f13fd](https://github.com/clarkbar-sys/tally/commit/18f13fd11f340db02c0d517b060150105d0982c4))
* **store:** server-side protocol persistence layer (S0) ([#106](https://github.com/clarkbar-sys/tally/issues/106)) ([856eb71](https://github.com/clarkbar-sys/tally/commit/856eb7101f026b208290cf6df381d0a746451b61))

## [0.6.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.5.0...tally-v0.6.0) (2026-07-20)


### Features

* **web:** applications — registered actors that author tallies ([#91](https://github.com/clarkbar-sys/tally/issues/91)) ([b762421](https://github.com/clarkbar-sys/tally/commit/b762421485ab52b24c7c8f2ada3a40cf96fdd7a4))
* **web:** modify-ops — let a tally (and an app) change an existing notch ([#93](https://github.com/clarkbar-sys/tally/issues/93)) ([199575b](https://github.com/clarkbar-sys/tally/commit/199575be7e4cd589e3740fa460ff1e2e2bdbe41e))

## [0.5.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.4.0...tally-v0.5.0) (2026-07-20)


### Features

* **web:** binary data in the substrate — add-blob change op ([#85](https://github.com/clarkbar-sys/tally/issues/85)) ([db57d7d](https://github.com/clarkbar-sys/tally/commit/db57d7dbccf90c1cf2642322859b72f3b1a4bca9))
* **web:** ledger — browse the data a merged tally wrote ([#84](https://github.com/clarkbar-sys/tally/issues/84)) ([cbb979e](https://github.com/clarkbar-sys/tally/commit/cbb979e473fea86b9a338613c3b71c7ef9ed82c9))
* **web:** tallies — reviewable proposals that merge into notches ([#82](https://github.com/clarkbar-sys/tally/issues/82)) ([c111d09](https://github.com/clarkbar-sys/tally/commit/c111d09936d0933d72b700488a50b6ea4c3533c0))

## [0.4.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.3.0...tally-v0.4.0) (2026-07-20)


### Features

* **web:** attach files and photos to notches ([#77](https://github.com/clarkbar-sys/tally/issues/77)) ([bd1a089](https://github.com/clarkbar-sys/tally/commit/bd1a089cf20d6e736025e189eec50207ec76d0b3))


### Bug Fixes

* **web:** make theme selector actually switch themes ([#79](https://github.com/clarkbar-sys/tally/issues/79)) ([ef46916](https://github.com/clarkbar-sys/tally/commit/ef4691679795a428cb0249d01a298f704bd83ce6))

## [0.3.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.2.0...tally-v0.3.0) (2026-07-20)


### Features

* run the app in in-memory demo mode (no IndexedDB) ([#72](https://github.com/clarkbar-sys/tally/issues/72)) ([a8e7332](https://github.com/clarkbar-sys/tally/commit/a8e733201006529c4054716b90c861588c243f88))
* **web:** add a theme switcher ([#65](https://github.com/clarkbar-sys/tally/issues/65)) ([dce468a](https://github.com/clarkbar-sys/tally/commit/dce468ac5f19f0638bb5224304c2febd75b20ecc))
* **web:** add the Paper and Gruvbox themes ([#66](https://github.com/clarkbar-sys/tally/issues/66)) ([c0c9788](https://github.com/clarkbar-sys/tally/commit/c0c97888b5925a0d01c71c7ad7994c02a2390164))
* **web:** add the Slate theme (calm dark) ([#62](https://github.com/clarkbar-sys/tally/issues/62)) ([37d7e71](https://github.com/clarkbar-sys/tally/commit/37d7e71fbaf8900e439acb0b73b1bea318b17079))
* **web:** cleaner, mobile-first UI with Paper as the default theme ([#68](https://github.com/clarkbar-sys/tally/issues/68)) ([d01a317](https://github.com/clarkbar-sys/tally/commit/d01a31702aa98c29c0daaaaa931d71e3689010cc))
* **web:** make the notch detail an append-only event log ([#74](https://github.com/clarkbar-sys/tally/issues/74)) ([6db3991](https://github.com/clarkbar-sys/tally/commit/6db3991b452c66014d5d58ccdb77917d7ced20aa))
* **web:** redesign notch detail as a GitHub-issue-style editor ([#71](https://github.com/clarkbar-sys/tally/issues/71)) ([8378455](https://github.com/clarkbar-sys/tally/commit/8378455cde697270344c5ff916c4a56a51af4bd9))

## [0.2.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.1.0...tally-v0.2.0) (2026-07-19)


### Features

* local-first tally app — generic container on IndexedDB ([#50](https://github.com/clarkbar-sys/tally/issues/50)) ([31da936](https://github.com/clarkbar-sys/tally/commit/31da93602c638ea88ebaa9d60dedecb851d60931))
* rename containers to notches, add sub-notches ([#52](https://github.com/clarkbar-sys/tally/issues/52)) ([da99922](https://github.com/clarkbar-sys/tally/commit/da99922dc2a6aa747171d2400733f7d092487498))
* **web:** close notches as done/not planned instead of deleting ([#54](https://github.com/clarkbar-sys/tally/issues/54)) ([d4fd57d](https://github.com/clarkbar-sys/tally/commit/d4fd57d41f770d96200d3cc89f22483ae5aaf690))
* **web:** let notches be re-parented to group existing ones ([#53](https://github.com/clarkbar-sys/tally/issues/53)) ([e01c8fb](https://github.com/clarkbar-sys/tally/commit/e01c8fb99837f8eba4509009569e0fcf2c0ea1ef))

## [0.1.0](https://github.com/clarkbar-sys/tally/compare/tally-v0.1.0...tally-v0.1.0) (2026-07-19)


### Miscellaneous Chores

* enforce conventional PR titles and cut v0.1.0 ([#42](https://github.com/clarkbar-sys/tally/issues/42)) ([6044381](https://github.com/clarkbar-sys/tally/commit/6044381715fa26fe144fd664db47b45757850327))
