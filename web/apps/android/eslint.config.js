// The workspace rules, plus: the Capacitor Android project is Java and
// Gradle. Its only JavaScript is what `cap sync` and Gradle copy in from
// node_modules (git-ignored), which is not ours to lint.
import { defineConfig } from 'eslint/config'

import workspace from '../../eslint.config.js'

export default defineConfig({ ignores: ['android/**'] }, workspace)
