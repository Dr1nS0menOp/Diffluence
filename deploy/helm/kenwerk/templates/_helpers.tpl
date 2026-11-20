{{/* SPDX-License-Identifier: Apache-2.0 */}}
{{- define "kenwerk.labels" -}}
app.kubernetes.io/name: kenwerk
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{- define "kenwerk.claim" -}}
{{- .Values.persistence.existingClaim | default (printf "%s-data" .Release.Name) }}
{{- end }}

{{/* The pod of one role. Call with a dict of root, role and name. */}}
{{- define "kenwerk.pod" -}}
{{- $root := .root -}}
metadata:
  labels:
    app.kubernetes.io/name: kenwerk
    app.kubernetes.io/instance: {{ $root.Release.Name }}
    app.kubernetes.io/component: {{ .name }}
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    runAsGroup: 65532
    fsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: kenwerk
      image: "{{ $root.Values.image.repository }}:{{ $root.Values.image.tag | default $root.Chart.AppVersion }}"
      imagePullPolicy: {{ $root.Values.image.pullPolicy }}
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities:
          drop: [ALL]
      ports:
        - name: http
          containerPort: 8080
      env:
        - name: KENWERK_ROLE
          value: {{ .role }}
        - name: KENWERK_PUBLIC_URL
          value: {{ required "publicUrl is required" $root.Values.publicUrl | quote }}
        - name: KENWERK_DATA_DIR
          value: /var/lib/kenwerk
        - name: KENWERK_MASTER_KEY_FILE
          value: /run/secrets/kenwerk/master-key
        - name: KENWERK_DATABASE_URL_FILE
          value: /run/secrets/kenwerk/database-url
        - name: KENWERK_MIGRATE_DATABASE_URL_FILE
          value: /run/secrets/kenwerk/migrate-database-url
        - name: KENWERK_METRICS_TOKEN
          valueFrom:
            secretKeyRef:
              name: {{ $root.Values.existingSecret }}
              key: metrics-token
              optional: true
        {{- if eq $root.Values.mode "split" }}
        - name: KENWERK_NATS_URL
          value: {{ required "natsUrl is required with mode split" $root.Values.natsUrl | quote }}
        {{- end }}
        {{- with $root.Values.trustedProxies }}
        - name: KENWERK_TRUSTED_PROXIES
          value: {{ . | quote }}
        {{- end }}
        {{- with $root.Values.extraEnv }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      livenessProbe:
        httpGet:
          path: /livez
          port: http
      readinessProbe:
        httpGet:
          path: /readyz
          port: http
      resources:
        {{- toYaml $root.Values.resources | nindent 8 }}
      volumeMounts:
        - name: data
          mountPath: /var/lib/kenwerk
        - name: tmp
          mountPath: /tmp
        - name: secrets
          mountPath: /run/secrets/kenwerk
          readOnly: true
  volumes:
    - name: tmp
      emptyDir: {}
    - name: data
      persistentVolumeClaim:
        claimName: {{ include "kenwerk.claim" $root }}
    - name: secrets
      secret:
        secretName: {{ $root.Values.existingSecret }}
{{- end }}
