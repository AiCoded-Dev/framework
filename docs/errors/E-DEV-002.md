# E-DEV-002: dev.yaml is not valid

`dev.yaml` has a syntax error or a field `aicoded dev` does not know.

**Fix:** correct the file at the reported line. The file looks like:

    workspaces:
      /home/me/apps:            # absolute path of the directory you run aicoded dev in
        port: 8080
        mysql: root:local-pw@unix(/var/run/mysqld/mysqld.sock)/   # a local MySQL 8, for apps with sqldb
        personas:
          - {name: alice, roles: [hotel-ops], groups: [lisbon]}
        apps:
          rooms:
            settings: {greeting: hi}
            secrets: {api_key: local-test-key}
