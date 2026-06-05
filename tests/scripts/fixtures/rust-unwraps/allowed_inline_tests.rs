#![forbid(unsafe_code)]

pub fn double(value: u64) -> u64 {
    value * 2
}

#[cfg(test)]
mod tests {
    use super::double;

    #[test]
    fn allows_expect_inside_cfg_test_module() {
        let value = Some(double(2)).expect("test helper");
        assert_eq!(value, 4);
    }
}
