// Pipeline of the example system (job "app", deploy/jenkins/casc/jobs.yaml).
//
//   every commit of the watched branch -> tests, images built and pushed
//   then                               -> deploy API, then frontend
//
// Jenkins has no access to the host's Docker: builds run on the rootless
// BuildKit container (builder "buildkit") and push to the local registry.
// Deploying is a restricted SSH request (forced command) that only accepts
// application services and images pinned by digest; the host waits for the
// health checks and rolls back on failure.
pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
        timeout(time: 30, unit: 'MINUTES')
    }

    environment {
        REPO_API = 'app/api'
        REPO_WEB = 'app/web'
    }

    stages {
        stage('Test') {
            parallel {
                stage('Backend') {
                    steps {
                        sh 'docker buildx build --target test --progress plain backend'
                    }
                }
                stage('Frontend') {
                    steps {
                        sh 'docker buildx build --target test --progress plain frontend'
                    }
                }
            }
        }

        stage('Build & push') {
            steps {
                sh '''
                    docker buildx build --push --progress plain \
                        --build-arg VERSION="${GIT_COMMIT}" \
                        -t "registry:5000/${REPO_API}:${GIT_COMMIT}" \
                        --metadata-file build-api.json backend
                    # Public value embedded in the browser bundle.
                    docker buildx build --push --progress plain \
                        --build-arg VITE_API_URL="https://${API_DOMAIN}" \
                        -t "registry:5000/${REPO_WEB}:${GIT_COMMIT}" \
                        --metadata-file build-web.json frontend
                '''
                script {
                    env.IMAGE_API = "127.0.0.1:5000/${env.REPO_API}:${env.GIT_COMMIT}@" +
                        readJSON(file: 'build-api.json')['containerimage.digest']
                    env.IMAGE_WEB = "127.0.0.1:5000/${env.REPO_WEB}:${env.GIT_COMMIT}@" +
                        readJSON(file: 'build-web.json')['containerimage.digest']
                }
                echo "api: ${env.IMAGE_API}\nweb: ${env.IMAGE_WEB}"
            }
        }

        stage('Deploy') {
            steps {
                // API first: if it fails and rolls back, the frontend never ships
                // a version that depends on the new API.
                sh '''
                    deploy() {
                        ssh -i /run/secrets/ci_deploy_ssh_key \
                            -o IdentitiesOnly=yes -o BatchMode=yes \
                            -o UserKnownHostsFile=/run/secrets/ci_deploy_known_hosts \
                            -o StrictHostKeyChecking=yes \
                            "deployer@${DEPLOY_SSH_HOST}" "$@"
                    }
                    deploy app app_api "${IMAGE_API}"
                    deploy app app_web "${IMAGE_WEB}"
                '''
            }
        }
    }

    post {
        always {
            sh 'rm -f build-api.json build-web.json'
        }
    }
}
