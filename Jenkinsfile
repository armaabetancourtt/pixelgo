pipeline {
  agent any
  options { timestamps(); timeout(time: 20, unit: 'MINUTES') }
  stages {
    stage('Checkout') { steps { checkout scm } }
    stage('Quality') { steps { sh 'bash scripts/check-analytics.sh' } }
    stage('Secret scan') {
      steps { sh 'docker run --rm -v "$PWD:/repo:ro" ghcr.io/gitleaks/gitleaks:latest detect --source /repo --no-git || exit 1' }
    }
  }
}
