# App Marketplace Template Parameters

This page explains how template parameters in the App Marketplace install dialog are filled, which values the platform fills in for you, and how parameters reach the running app. For the install steps, see [App Marketplace](../use/app-marketplace).

## Parameter types

The install dialog lists the parameters defined by the template. Each parameter may have these properties:

| Property | Meaning | When left empty |
| --- | --- | --- |
| Has a default | The template provides a recommended value | The default is used |
| Auto-generated | Suitable for passwords, keys, and tokens | The platform generates a 43-character random value (character set `A-Z a-z 0-9 - _`) |
| Required | The install needs a value | The install fails if neither a default nor generation yields a value |
| Secret | The value is written only to secret storage | An optional secret left empty is not injected |

Rules:

- Leading and trailing whitespace is trimmed from entered values.
- Secret parameters are injected only as Secrets. They are not written to plain environment variables, the install parameter snapshot, or the install result, and are never shown after install.
- All components of a multi-component template share one set of resolved parameters. For example, a database password is generated once and injected into both the database component and the main app.
- A missing required parameter returns `app_template.value_required`; fill it in and install again. A failed random value generation returns `app_template.secret_generation_failed`, and the install stops instead of falling back to a fixed value.

## How parameters reach the app

Templates write parameters into a component's environment variables, encrypted variables, configuration files, or secret files. After install these become ordinary deploy config settings that you can view and change (encrypted values are not shown). Changes require a new release.

- When a template uses a password inside a connection string, the password is URL-escaped (for example Neo Blog's `DB_URL` and `REDIS_URL`), so passwords containing `@`, `:`, or `/` still work.
- Before changing an already initialized database password, password salt, or similar parameter, confirm that the app supports it. Most databases read the initial password only when the data volume is first initialized.

## Service addresses in multi-component templates

In multi-component templates, the platform fills in the addresses between components at install time:

- Settings that reference another component's address are replaced with that component's service name in the project space, for example `blog-postgres`.
- Each dependency also injects `<COMPONENT>_HOST` and `<COMPONENT>_PORT` environment variables, with the component key in upper case, for example `POSTGRES_HOST` and `POSTGRES_PORT`. The `_HOST` value is the in-cluster domain `<service>.<namespace>.svc.cluster.local`.

These variables come from service relations. To define a variable with the same name in a deploy config, adjust the service relation first; otherwise the release fails with a variable name conflict.
