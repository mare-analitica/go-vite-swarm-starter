#!/bin/sh
# Development only: creates the application bucket and credentials that can
# read and write that bucket and nothing else. Idempotent.
set -eu

mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc mb --ignore-existing "local/$APP_BUCKET" >/dev/null

policy=/tmp/app-policy.json
cat > "$policy" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": ["arn:aws:s3:::${APP_BUCKET}/*"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
      "Resource": ["arn:aws:s3:::${APP_BUCKET}"]
    }
  ]
}
EOF

mc admin policy create local "${APP_BUCKET}-rw" "$policy" >/dev/null
mc admin user add local "$APP_ACCESS_KEY" "$APP_SECRET_KEY" >/dev/null
mc admin policy attach local "${APP_BUCKET}-rw" --user "$APP_ACCESS_KEY" >/dev/null 2>&1 || true
echo "bucket '${APP_BUCKET}' and scoped credentials ready"
