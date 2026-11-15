// Domain: classroom (planned). Detailed by the H8 design.
PH.domain({
  id: 'classroom', parent: null, kind: 'traditional', status: 'planned',
  name: 'Classroom',
  summary: 'Classes and private family groups: members, materials, context, activities and grades.',
  responsibilities: [
    'Keep classes: name, description, invitation code, pending requests, activity history, closing, archiving and deletion with their retention windows.',
    'Keep private family groups with the same machinery as classes, without grades or materials.',
    'Keep enrollment and the student data inside each class: name, alias, internal identifier; removal revokes access without deleting results that must be kept.',
    'Keep materials (private by default, folders and tags, replacement affects future generations) and the pedagogical context given to the AI.',
    'Keep the teacher library: favorites by reference, editable copies only of templates, free organization.',
    'Keep class activities: a prepared use of a game, pinning version and configuration; content or grading changes after use mean another activity.',
    'Own grades: attempts, states, scales declared before playing, review, corrections, batch publication, optional final grade with explicit equivalences, XLSX export.',
    'Validate that a user belongs to the class or group of a Session when orchestration asks.',
    'Duplicate a class: structure, authorized materials and context; never students, grades or personal context.'
  ],
  doesNotKnow: [
    'How games execute or what a Session looks like inside (play).',
    'Authorization evidence of minors (compliance keeps it; classroom blocks the student when coverage is missing).',
    'Subscriptions and payment state (plan limits arrive from the caller).',
    'Student credentials themselves (identity issues and recognizes them).'
  ],
  references: [
    'Identity: students, teachers and group hosts (user identifiers).',
    'Game: games in libraries and activities (game identifiers).',
    'Play: Sessions of each activity and their results (session identifiers).',
    'Compliance: authorization coverage of each student (coverage identifier).'
  ],
  sources: ['B: classroom', 'P02', 'P03§2-7', 'P07', 'P08', 'C-02']
});
