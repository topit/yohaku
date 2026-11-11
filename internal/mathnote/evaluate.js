// Yohaku's deliberately bounded scalar calculator, not Heynote code.
// Called only in a worker-owned VM. Nothing exposes Go, files or networking.
(function () {
    const functions = new Set([
        'abs', 'sqrt', 'cbrt', 'sin', 'cos', 'tan', 'asin', 'acos', 'atan', 'atan2',
        'sinh', 'cosh', 'tanh', 'log', 'log10', 'log2', 'exp', 'round', 'floor',
        'ceil', 'min', 'max', 'pow', 'mod', 'sign'
    ]);
    const operators = new Set(['+', '-', '*', '/', '^', '%', 'to', 'in']);
    let scope;
    let prev;
    globalThis.beginBlock = function () {
        scope = new Map();
        prev = undefined;
    };
    function numberLiteral(node) {
        if (node.isConstantNode) return typeof node.value === 'number';
        if (node.isParenthesisNode) return numberLiteral(node.content);
        return node.isOperatorNode && ['+', '-'].includes(node.op) &&
            node.args.length === 1 && numberLiteral(node.args[0]);
    }
    function prose(source) {
        const text = source.split('#')[0].trim();
        // An undefined word is usually a heading, not a broken formula.
        // Keep explicit math, malformed numbers and unsupported code visible.
        const mathSyntax = /[+\-*/^%=()[\]{}<>:;,"']|\.[A-Za-z_]/;
        if (/[A-Za-z_\u0080-\uffff]/.test(text) && !mathSyntax.test(text)) return true;
        // A date-like prefix followed by a reminder is a note, not a
        // subtraction. Do not generalize this to arithmetic prefixes:
        // '2 + unknown' and '1 + * missing' must still report their errors.
        const suffix = /^\d{1,4}[-/.]\d{1,2}(?:[-/.]\d{1,2})?\s+([A-Za-z_\u0080-\uffff].*)$/.exec(text);
        return !!suffix && !mathSyntax.test(suffix[1]);
    }
    function evaluate(source, explicit) {
        if (!source.trim() || source.trimStart().startsWith('#')) return {};
        try {
            let node;
            try {
                node = math.parse(source);
            } catch (error) {
                // The parser reached EOF while expecting another token (an
                // operand, closing parenthesis, etc.). Keep partial input
                // quiet; syntax errors before EOF remain visible.
                if (error.name === 'SyntaxError' &&
                    (error.char === source.length + 1 || error.message.startsWith('Digit expected, got ""'))) return {};
                throw error;
            }
            let count = 0;
            node.traverse(function (n) {
                if (++count > 256) throw Error('Expression is too complex (256 nodes maximum)');
                if (n.isConstantNode && typeof n.value === 'number') return;
                if (n.isSymbolNode || n.isParenthesisNode) return;
                if (n.isAssignmentNode && n === node && !n.index) return;
                if (n.isOperatorNode && operators.has(n.op)) return;
                if (n.isFunctionNode && n.fn.isSymbolNode && functions.has(n.fn.name)) return;
                throw Error('Not supported in this scalar Math version: ' + n.type);
            });
            scope.set('prev', prev);
            // Validate before committing an assignment. A rejected function
            // value must not smuggle e.g. ones under an allowed function name.
            const expression = node.isAssignmentNode ? node.value : node;
            // Decimal arithmetic must not carry binary tails into a later
            // subtraction or variable. Keep units/functions/complex values on
            // their existing math.js path; promote only scalar arithmetic.
            let decimal = true;
            expression.traverse(function (n) {
                if (n.isConstantNode || n.isParenthesisNode) return;
                if (n.isOperatorNode && ['+', '-', '*', '/', '^', '%'].includes(n.op)) return;
                if (n.isSymbolNode && scope.has(n.name) &&
                    (typeof scope.get(n.name) === 'number' || math.isBigNumber(scope.get(n.name)))) return;
                decimal = false;
            });
            let value;
            if (decimal) {
                const precise = expression.transform(function (n) {
                    if (n.isConstantNode) return new math.ConstantNode(math.bignumber(n.value));
                    if (n.isSymbolNode) return new math.ConstantNode(math.bignumber(scope.get(n.name)));
                    return n;
                });
                value = precise.compile().evaluate(scope);
            } else {
                const numbers = new Map(scope);
                numbers.forEach(function (v, k) { if (math.isBigNumber(v)) numbers.set(k, v.toNumber()); });
                value = expression.compile().evaluate(numbers);
            }
            if (value === undefined) return {};
            if (typeof value !== 'number' && !math.isBigNumber(value) && !math.isUnit(value) && !math.isComplex(value)) {
                throw Error('Expected a scalar number or unit');
            }
            const copy = math.format(value, {precision: 14, upperExp: 14, lowerExp: -10});
            if (copy.length > 512 || /NaN|Infinity/.test(copy)) throw Error('Result is not finite or is too large');
            if (node.isAssignmentNode) scope.set(node.object.name, value);
            prev = value;
            // Keep literals usable by prev, without echoing every amount.
            // Assignments and an explicit '=' remain visible/completable.
            if (!explicit && numberLiteral(node)) return {};
            return {display: copy, copy: copy};
        } catch (error) {
            return {error: String(error.message || error).slice(0, 180)};
        }
    }
    globalThis.calculate = function (source) {
        let complete = false;
        let annotation;
        // An assignment's first '=' is not a result trigger. An additional
        // '=' (or an equation such as 2+3=5) annotates the expression on its
        // left, never assigns to it. Recompute rather than trusting that text.
        const assignment = /^\s*[A-Za-z_][A-Za-z_0-9]*\s*=(?!=)/.exec(source);
        const from = assignment ? assignment[0].length : 0;
        const code = source.split('#')[0];
        const equals = code.indexOf('=', from);
        if (equals >= 0 && source[equals+1] !== '=' && source[equals-1] !== '=') {
            annotation = code.slice(equals+1).trim();
            complete = annotation === '' && !source.includes('#');
            source = source.slice(0, equals);
        }
        const explicit = !!assignment || annotation !== undefined;
        let result = evaluate(source, explicit);
        // Prefer real math, including variables and implicit multiplication.
        // Only then try a whitespace-separated description followed by a
        // number, parenthesized formula or explicit xN/prev reference. Never
        // salvage malformed arithmetic by silently dropping its left side.
        if (result.error) {
            const prefix = assignment ? source.slice(0, from) : '';
            const body = source.slice(from);
            const spaces = /\s+/g;
            let match;
            while ((match = spaces.exec(body))) {
                const label = body.slice(0, match.index).trim();
                const formula = body.slice(spaces.lastIndex);
                // With an ASCII name, '+ 1' could be an ordinary binary
                // operation on an undefined variable, not a signed amount.
                if (!/[A-Za-z_\u0080-\uffff]/.test(label) ||
                    !/^[A-Za-z_0-9\u0080-\uffff\s:：]+$/.test(label) ||
                    !/^(?:[0-9.(]|x[0-9]+\b|prev\b)/.test(formula) &&
                    !(/[\u0080-\uffff]/.test(label) && /^[+-]\s*(?:[0-9.(]|x[0-9]+\b|prev\b)/.test(formula))) continue;
                result = evaluate(prefix + formula, explicit);
                break;
            }
        }
        if (result.error && !explicit && prose(source)) return {};
        if (complete && result.copy) result.complete = true;
        if (annotation && annotation === result.copy) result.written = true;
        return result;
    };
    // math.js lazily builds its parser/typed functions. Initialize trusted
    // paths before timed, cancellable user jobs, rather than interrupting a
    // lazy initialization halfway through and leaving a poisoned VM.
    functions.forEach(function (name) { void math[name]; });
    beginBlock();
    calculate('12*3+1/2-3^2%2');
    calculate('sin(90 deg)');
    calculate('13 inches in cm');
    beginBlock();
})();
